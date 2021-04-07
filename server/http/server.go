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

type NervexServer struct {
	KubeClient         *kubernetes.Clientset
	DynamicClient      dynamic.Interface
	Log                logr.Logger
	alconfigDyInformer cache.SharedIndexInformer
	njDyInformer       cache.SharedIndexInformer
	alconfig           string
}

type NervexJobRequest struct {
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

type NervexJobResponse struct {
	Namespace   string   `json:"namespace"`
	Coordinator string   `json:"coordinator"`
	Aggregator  string   `json:"aggregator"`
	Actors      []string `json:"actors"`
	Learners    []string `json:"learners"`
}

func NewNervexServer(
	kubeClient *kubernetes.Clientset,
	dynamicClient dynamic.Interface,
	log logr.Logger,
	alconfigDyInformer cache.SharedIndexInformer,
	njDyInformer cache.SharedIndexInformer,
	alconfig string) *NervexServer {

	return &NervexServer{
		KubeClient:         kubeClient,
		DynamicClient:      dynamicClient,
		Log:                log,
		alconfigDyInformer: alconfigDyInformer,
		njDyInformer:       njDyInformer,
		alconfig:           alconfig,
	}
}

func (s *NervexServer) Start() error {
	log := s.Log.WithName("NervexServer")
	http.HandleFunc("/api/v1alpha1/add", s.Add)
	http.HandleFunc("/api/v1alpha1/delete", s.Delete)
	http.HandleFunc("/healthz", healthz)

	log.Info("Start listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		return err
	}
	return nil
}

func (s *NervexServer) Add(w http.ResponseWriter, r *http.Request) {
	log := s.Log.WithName("NervexServer")

	// parse request body
	var njreq NervexJobRequest
	err := json.NewDecoder(r.Body).Decode(&njreq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("request body: ", "request", njreq)

	// get ALConfig
	obj, exists, err := s.alconfigDyInformer.GetIndexer().GetByKey(s.alconfig)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get ALConfig: %s", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	if !exists {
		http.Error(w, fmt.Sprintf("ActorLearnerConfig: %s not exists", s.alconfig), http.StatusInternalServerError)
		return
	}

	alconfig := &nervexv1alpha1.ActorLearnerConfig{}
	err = nervexutil.GetObjectFromUnstructured(obj, alconfig)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// get coordinator
	coordinator, err := s.KubeClient.CoreV1().Pods(njreq.Namespace).Get(context.Background(), njreq.Coordinator, metav1.GetOptions{})
	if err != nil {
		errMsg := fmt.Sprintf("failed to get coordinator: %s", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	ownRefers := coordinator.GetOwnerReferences()
	if len(ownRefers) == 0 {
		errMsg := fmt.Sprintf("orphaned coordinator found: %s/%s, please ensure which NervexJob it belongs to", coordinator.Namespace, coordinator.Name)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}
	var ownRefer metav1.OwnerReference
	for _, ref := range ownRefers {
		if ref.Kind == "NerveXJob" {
			ownRefer = ref
		}
	}

	// get NervexJob
	njKey := fmt.Sprintf("%s/%s", njreq.Namespace, ownRefer.Name)
	obj, exists, err = s.njDyInformer.GetIndexer().GetByKey(njKey)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get NervexJob: %s", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	if !exists {
		http.Error(w, fmt.Sprintf("NerveXJob: %s not exists in cache", njKey), http.StatusInternalServerError)
		return
	}

	nj := &nervexv1alpha1.NerveXJob{}
	err = nervexutil.GetObjectFromUnstructured(obj, nj)
	if err != nil {
		errMsg := fmt.Sprintf("failed to convert NervexJob: %s", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	// create actors and learners
	actors, learners, err := s.createActorsAndLearnersFromALConfig(alconfig, &njreq, ownRefer)
	if err != nil {
		errMsg := fmt.Sprintf("failed create actors and learners: %s", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	rep := NervexJobResponse{
		Namespace:   njreq.Namespace,
		Coordinator: njreq.Coordinator,
		Actors:      actors,
		Learners:    learners,
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

func (s *NervexServer) createActorsAndLearnersFromALConfig(
	alconfig *nervexv1alpha1.ActorLearnerConfig,
	njreq *NervexJobRequest,
	ownRefer metav1.OwnerReference) ([]string, []string, error) {

	// create actors
	actorTemplate := alconfig.Spec.Actor.Template
	actors, err := s.createPodsAndServices(&actorTemplate, ownRefer, njreq,
		nervexutil.ActorName, nervexutil.DefaultActorContainerName, nervexutil.DefaultActorPortName, nervexutil.DefaultActorPort)

	if err != nil {
		return nil, nil, err
	}

	// create learners
	learnerTemplate := alconfig.Spec.Learner.Template
	learners, err := s.createPodsAndServices(&learnerTemplate, ownRefer, njreq,
		nervexutil.LearnerName, nervexutil.DefaultLearnerContainerName, nervexutil.DefaultLearnerPortName, nervexutil.DefaultLearnerPort)

	if err != nil {
		return nil, nil, err
	}

	return actors, learners, nil
}

func (s *NervexServer) createPodsAndServices(
	template *corev1.PodTemplateSpec,
	ownRefer metav1.OwnerReference,
	njreq *NervexJobRequest,
	replicaType, containerName, portName string, defaultPort int32) ([]string, error) {

	log := s.Log.WithName("NervexServer")
	coordinator := njreq.Coordinator
	ns := njreq.Namespace

	portEnv := ""
	resources := ResourceQuantity{}
	if replicaType == nervexutil.ActorName {
		resources = njreq.Actors
		portEnv = "ACTOR_PORT"
	} else if replicaType == nervexutil.LearnerName {
		resources = njreq.Learners
		portEnv = "LEARNER_PORT"
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
		log.Info("no port found, use default port", "port", port)
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
		template.Spec.Containers[i].Resources.Limits[corev1.ResourceName("nvidia.com/gpu")] = resources.Gpu
		template.Spec.Containers[i].Resources.Requests[corev1.ResourceName("nvidia.com/gpu")] = resources.Gpu
	}
}

func (s *NervexServer) Delete(w http.ResponseWriter, r *http.Request) {
	log := s.Log.WithName("NervexServer")

	// parse request body
	var njreq NervexJobRequest
	err := json.NewDecoder(r.Body).Decode(&njreq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("delete request body: ", "request", njreq)

	// get coordinator
	coordinator, err := s.KubeClient.CoreV1().Pods(njreq.Namespace).Get(context.Background(), njreq.Coordinator, metav1.GetOptions{})
	if err != nil {
		errMsg := fmt.Sprintf("failed to get coordinator: %s", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	ownRefers := coordinator.GetOwnerReferences()
	if len(ownRefers) == 0 {
		errMsg := fmt.Sprintf("orphaned coordinator found: %s/%s, please ensure which NervexJob it belongs to", coordinator.Namespace, coordinator.Name)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}
	var ownRefer metav1.OwnerReference
	for _, ref := range ownRefers {
		if ref.Kind == "NerveXJob" {
			ownRefer = ref
		}
	}

	// get NervexJob
	njKey := fmt.Sprintf("%s/%s", njreq.Namespace, ownRefer.Name)
	obj, exists, err := s.njDyInformer.GetIndexer().GetByKey(njKey)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get NervexJob: %s", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	if !exists {
		http.Error(w, fmt.Sprintf("NerveXJob: %s not exists in cache", njKey), http.StatusInternalServerError)
		return
	}

	nj := &nervexv1alpha1.NerveXJob{}
	err = nervexutil.GetObjectFromUnstructured(obj, nj)
	if err != nil {
		errMsg := fmt.Sprintf("failed to convert NervexJob: %s", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	// list pods
	pods, err := s.listPods(njreq.Namespace, ownRefer)
	if err != nil {
		errMsg := fmt.Sprintf("failed to list pod through ownrefernece: %s", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	// classify pods
	actors, learners, _, err := nervexutil.ClassifyPods(pods)
	if err != nil {
		errMsg := fmt.Sprintf("unable to classify pods: %s", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
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

	rep := NervexJobResponse{
		Namespace:   njreq.Namespace,
		Coordinator: njreq.Coordinator,
		Actors:      delActors,
		Learners:    delLearners,
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

func (s *NervexServer) listPods(ns string, ownRefer metav1.OwnerReference) ([]*corev1.Pod, error) {
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: nervexutil.GenLabels(ownRefer.Name),
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

func (s *NervexServer) DeletePodsAndServices(pods []*corev1.Pod, njreq *NervexJobRequest, replicaType string) ([]string, error) {
	results := []string{}
	resources := ResourceQuantity{}
	ns := njreq.Namespace

	if replicaType == nervexutil.ActorName {
		resources = njreq.Actors
	} else if replicaType == nervexutil.LearnerName {
		resources = njreq.Learners
	}

	for _, pod := range pods {

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
		// break if enough
		if len(results) == resources.Replicas {
			break
		}
	}

	return results, nil
}
