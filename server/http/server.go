package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
	nervexutil "go-sensephoenix.sensetime.com/nervex-operator/utils"
)

type NerveXServer struct {
	KubeClient         *kubernetes.Clientset
	DynamicClient      dynamic.Interface
	Log                logr.Logger
	alconfigDyInformer cache.SharedIndexInformer
	njDyInformer       cache.SharedIndexInformer
	alconfig           string
}

type NerveXJobRequest struct {
	Namespace   string           `json:"namespace"`
	Coordinator string           `json:"coordinator"`
	Actors      ResourceQuantity `json:"actors"`
	Learners    ResourceQuantity `json:"learners"`
}

type ResourceQuantity struct {
	Replicas int               `json:"replicas"`
	Cpu      resource.Quantity `json:"cpus"`
	Gpu      resource.Quantity `json:"gpus"`
	Memory   resource.Quantity `json:"memory"`
}

type NerveXJobResponse struct {
	Namespace   string   `json:"namespace"`
	Coordinator string   `json:"coordinator"`
	Aggregator  string   `json:"aggregator"`
	Actors      []string `json:"actors"`
	Learners    []string `json:"learners"`
}

func NewNerveXServer(
	kubeClient *kubernetes.Clientset,
	dynamicClient dynamic.Interface,
	log logr.Logger,
	alconfigDyInformer cache.SharedIndexInformer,
	njDyInformer cache.SharedIndexInformer,
	alconfig string) *NerveXServer {

	return &NerveXServer{
		KubeClient:         kubeClient,
		DynamicClient:      dynamicClient,
		Log:                log,
		alconfigDyInformer: alconfigDyInformer,
		njDyInformer:       njDyInformer,
		alconfig:           alconfig,
	}
}

func (s *NerveXServer) Start() error {
	log := s.Log.WithName("NerveXServer")
	http.HandleFunc("/api/v1alpha1/add", s.Add)
	http.HandleFunc("/api/v1alpha1/delete", s.Delete)
	http.HandleFunc("/healthz", healthz)

	log.Info("Start listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		return err
	}
	return nil
}

func (s *NerveXServer) Add(w http.ResponseWriter, r *http.Request) {
	log := s.Log.WithName("NerveXServer")

	// parse request body
	var njreq NerveXJobRequest
	err := json.NewDecoder(r.Body).Decode(&njreq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("request body: ", "request", njreq)

	// get ALConfig
	alconfig, err := s.getALConfig()
	if err != nil {
		log.Error(err, "failed to get ALConfig", "alconfig", s.alconfig)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// get ownReference of request coordinator
	ownRefer, err := s.getOwnerReference(njreq)
	if err != nil {
		log.Error(err, "failed to get OwnerReference of coordinator %s", nervexutil.NamespacedName(njreq.Namespace, njreq.Coordinator))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, _, _, ag, err := s.listActorsAndLearners(njreq.Namespace, ownRefer.Name)
	if err != nil {
		log.Error(err, "failed to list actors and learners")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	needAgg := false
	if ag == nil {
		needAgg = true
	}

	// create actors and learners
	actors, learners, aggregator, err := s.createActorsAndLearnersFromALConfig(alconfig, &njreq, *ownRefer, needAgg)
	if err != nil {
		errMsg := fmt.Sprintf("failed create actors and learners: %s", err)
		log.Error(err, "failed create actors and learners")
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	// write response
	writeResponse(w, njreq.Namespace, njreq.Coordinator, actors, learners, aggregator)
}

func (s *NerveXServer) getOwnerReference(njreq NerveXJobRequest) (*metav1.OwnerReference, error) {
	// get coordinator
	coordinator, err := s.KubeClient.CoreV1().Pods(njreq.Namespace).Get(context.Background(), njreq.Coordinator, metav1.GetOptions{})
	if err != nil {
		errMsg := fmt.Sprintf("failed to get coordinator: %s", err)
		return nil, fmt.Errorf(errMsg)
	}

	var ownRefer metav1.OwnerReference
	ownRefers := coordinator.GetOwnerReferences()
	ownByNerveX := false
	for _, ref := range ownRefers {
		if ref.Kind == nervexv1alpha1.KindNerveXJob {
			ownRefer = ref
			ownByNerveX = true
		}
	}
	if !ownByNerveX {
		errMsg := "coordinator is not owned by NerveXJob"
		return nil, fmt.Errorf(errMsg)
	}

	// get NerveXJob
	njKey := fmt.Sprintf("%s/%s", njreq.Namespace, ownRefer.Name)
	_, exists, err := s.njDyInformer.GetIndexer().GetByKey(njKey)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get NerveXJob: %s", err)
		return nil, fmt.Errorf(errMsg)
	}

	if !exists {
		errMsg := fmt.Sprintf("NerveXJob: %s not exists in cache", njKey)
		return nil, fmt.Errorf(errMsg)
	}

	return &ownRefer, nil
}

func writeResponse(w http.ResponseWriter, ns, coordinator string, actors, learners []string, aggregator string) {
	rep := NerveXJobResponse{
		Namespace:   ns,
		Coordinator: coordinator,
		Actors:      actors,
		Learners:    learners,
		Aggregator:  aggregator,
	}
	w.Header().Set("Conten-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	repJson, err := json.Marshal(rep)
	if err != nil {
		errMsg := fmt.Sprintf("failed to marshal json: %s", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}
	_, err = w.Write(repJson)
	if err != nil {
		errMsg := fmt.Sprintf("failed to write json: %s", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
	}
}

func (s *NerveXServer) getALConfig() (*nervexv1alpha1.ActorLearnerConfig, error) {
	obj, exists, err := s.alconfigDyInformer.GetIndexer().GetByKey(s.alconfig)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get ALConfig: %s", err)
		return nil, fmt.Errorf(errMsg)
	}
	if !exists {
		errMsg := fmt.Sprintf("ActorLearnerConfig: %s not exists", s.alconfig)
		return nil, fmt.Errorf(errMsg)
	}

	alconfig := &nervexv1alpha1.ActorLearnerConfig{}
	err = nervexutil.GetObjectFromUnstructured(obj, alconfig)
	if err != nil {
		return nil, err
	}

	return alconfig, nil
}

func (s *NerveXServer) listActorsAndLearners(ns, jobName string) (
	actors []*corev1.Pod, learners []*corev1.Pod, coordinator *corev1.Pod, aggregator *corev1.Pod, err error) {
	// list pods that belong to the NerveXJob
	pods, err := s.listPods(ns, jobName)
	if err != nil {
		return
	}

	// classify pods
	actors, learners, coordinator, aggregator, err = nervexutil.ClassifyPods(pods)
	if err != nil {
		return
	}
	return
}

func (s *NerveXServer) createActorsAndLearnersFromALConfig(
	alconfig *nervexv1alpha1.ActorLearnerConfig,
	njreq *NerveXJobRequest,
	ownRefer metav1.OwnerReference,
	needAgg bool) ([]string, []string, string, error) {

	// create actors
	actorTemplate := alconfig.Spec.Actor.Template
	actors, err := s.createPodsAndServices(&actorTemplate, ownRefer, njreq,
		nervexutil.ActorName, nervexutil.DefaultActorContainerName, nervexutil.DefaultActorPortName, nervexutil.DefaultActorPort)

	if err != nil {
		return nil, nil, "", err
	}

	// create learners
	learnerTemplate := alconfig.Spec.Learner.Template
	learners, err := s.createPodsAndServices(&learnerTemplate, ownRefer, njreq,
		nervexutil.LearnerName, nervexutil.DefaultLearnerContainerName, nervexutil.DefaultLearnerPortName, nervexutil.DefaultLearnerPort)

	if err != nil {
		return nil, nil, "", err
	}

	if !needAgg {
		return actors, learners, "", nil
	}
	// create aggregator
	aggregatorTemplate := alconfig.Spec.Learner.Template
	aggregators, err := s.createPodsAndServices(&aggregatorTemplate, ownRefer, njreq,
		nervexutil.AggregatorName, nervexutil.DefaultAggregatorContainerName, nervexutil.DefaultAggregatorPortName, nervexutil.DefaultAggregatorPort)
	if err != nil {
		return nil, nil, "", err
	}
	aggregator := aggregators[0]

	return actors, learners, aggregator, nil
}

func (s *NerveXServer) createPodsAndServices(
	template *corev1.PodTemplateSpec,
	ownRefer metav1.OwnerReference,
	njreq *NerveXJobRequest,
	replicaType, containerName, portName string, defaultPort int32) ([]string, error) {

	log := s.Log.WithName("NerveXServer")
	coordinator := njreq.Coordinator
	ns := njreq.Namespace

	portEnv := ""
	resources := ResourceQuantity{}
	switch replicaType {
	case nervexutil.ActorName:
		resources = njreq.Actors
		portEnv = "ACTOR_PORT"
	case nervexutil.LearnerName:
		resources = njreq.Learners
		portEnv = "LEARNER_PORT"
	case nervexutil.AggregatorName:
		resources = ResourceQuantity{Replicas: 1}
		portEnv = "AGGREGATOR_PORT"
	}

	// add labels to pod template
	labels := nervexutil.GenLabels(ownRefer.Name)
	labels[nervexutil.ReplicaTypeLabel] = replicaType
	nervexutil.AddLabelsToPodTemplate(template, labels)

	// setup pod template
	template.GenerateName = fmt.Sprintf("%s-%s-", coordinator, replicaType)
	template.SetOwnerReferences([]metav1.OwnerReference{ownRefer})
	template.Namespace = ns
	// set pod resource
	SetPodTemplateResources(template, resources, containerName)
	// get pod port
	port, ok := nervexutil.GetPortFromPodTemplate(template, containerName, portName)
	if !ok {
		port = defaultPort
		log.Info("no port found, use default port", "container", containerName, "port", port)
		nervexutil.SetPodTemplatePort(template, containerName, portName, port)
	}

	// add env
	envs := make(map[string]string)
	envs[portEnv] = fmt.Sprintf("%d", port)
	nervexutil.SetPodTemplateEnv(template, envs)

	// build pod
	pod := nervexutil.BuildPodFromTemplate(template.DeepCopy())

	// build service
	svc := nervexutil.BuildService(labels, port, portName)
	svc.Labels = labels
	svc.SetOwnerReferences([]metav1.OwnerReference{ownRefer})

	results := []string{}
	// create pods and services
	for i := 0; i < resources.Replicas; i++ {
		tempPod := pod.DeepCopy()
		newPod, err := s.KubeClient.CoreV1().Pods(ns).Create(context.Background(), tempPod, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}

		tempSvc := svc.DeepCopy()
		tempSvc.Name = newPod.Name
		_, err = s.KubeClient.CoreV1().Services(ns).Create(context.Background(), tempSvc, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}

		result := fmt.Sprintf("%s.%s:%d", tempSvc.Name, ns, port)
		results = append(results, result)
	}

	return results, nil
}

func SetPodTemplateResources(template *corev1.PodTemplateSpec, resources ResourceQuantity, containerName string) {
	for i := range template.Spec.Containers {
		if template.Spec.Containers[i].Name != containerName {
			continue
		}
		if template.Spec.Containers[i].Resources.Limits == nil {
			template.Spec.Containers[i].Resources.Limits = make(corev1.ResourceList)
		}
		if template.Spec.Containers[i].Resources.Requests == nil {
			template.Spec.Containers[i].Resources.Requests = make(corev1.ResourceList)
		}

		// cpu and memory must not be zero
		if !resources.Cpu.IsZero() {
			template.Spec.Containers[i].Resources.Limits[corev1.ResourceCPU] = resources.Cpu
			template.Spec.Containers[i].Resources.Requests[corev1.ResourceCPU] = resources.Cpu
		}
		if !resources.Memory.IsZero() {
			template.Spec.Containers[i].Resources.Limits[corev1.ResourceMemory] = resources.Memory
			template.Spec.Containers[i].Resources.Requests[corev1.ResourceMemory] = resources.Memory
		}
		if !resources.Gpu.IsZero() {
			template.Spec.Containers[i].Resources.Limits[corev1.ResourceName("nvidia.com/gpu")] = resources.Gpu
			template.Spec.Containers[i].Resources.Requests[corev1.ResourceName("nvidia.com/gpu")] = resources.Gpu
		}
	}
}

func (s *NerveXServer) Delete(w http.ResponseWriter, r *http.Request) {
	log := s.Log.WithName("NerveXServer")

	// parse request body
	var njreq NerveXJobRequest
	err := json.NewDecoder(r.Body).Decode(&njreq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("delete request body: ", "request", njreq)

	// get ownReference of the request coordinator
	ownRefer, err := s.getOwnerReference(njreq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// list pods that belong to the NerveXJob
	actors, learners, _, _, err := s.listActorsAndLearners(njreq.Namespace, ownRefer.Name)
	if err != nil {
		log.Error(err, "failed to list actors and learners")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// delete actor pods
	delActors, err := s.DeletePodsAndServices(actors, &njreq, nervexutil.ActorName)
	if err != nil {
		errMsg := fmt.Sprintf("failed to delete actor: %s", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	// delete learner pods
	delLearners, err := s.DeletePodsAndServices(learners, &njreq, nervexutil.LearnerName)
	if err != nil {
		errMsg := fmt.Sprintf("failed to delete learner: %s", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	// write response
	writeResponse(w, njreq.Namespace, njreq.Coordinator, delActors, delLearners, "")
}

func (s *NerveXServer) listPods(ns string, jobName string) ([]*corev1.Pod, error) {
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: nervexutil.GenLabels(jobName),
	})
	if err != nil {
		return nil, err
	}

	podList, err := s.KubeClient.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{LabelSelector: labelSelector.String()})
	if err != nil {
		return nil, err
	}

	pods := []*corev1.Pod{}
	for _, pod := range podList.Items {
		pods = append(pods, pod.DeepCopy())
	}

	return pods, nil
}

func (s *NerveXServer) DeletePodsAndServices(pods []*corev1.Pod, njreq *NerveXJobRequest, replicaType string) ([]string, error) {
	results := []string{}
	resources := ResourceQuantity{}
	ns := njreq.Namespace

	switch replicaType {
	case nervexutil.ActorName:
		resources = njreq.Actors
	case nervexutil.LearnerName:
		resources = njreq.Learners
	}

	for _, pod := range pods {
		// break if enough
		if len(results) >= resources.Replicas {
			break
		}

		err := s.KubeClient.CoreV1().Pods(njreq.Namespace).Delete(context.Background(), pod.Name, metav1.DeleteOptions{})
		if err != nil {
			return results, err
		}
		err = s.KubeClient.CoreV1().Services(njreq.Namespace).Delete(context.Background(), pod.Name, metav1.DeleteOptions{})
		if err != nil {
			return results, err
		}

		result := fmt.Sprintf("%s.%s", pod.Name, ns)
		results = append(results, result)

	}

	return results, nil
}
