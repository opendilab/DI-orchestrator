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
	nervexutil "go-sensephoenix.sensetime.com/nervex-operator/util"
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
	Number int               `json:"number"`
	CPUs   resource.Quantity `json:"cpus"`
	GPUs   resource.Quantity `json:"gpus"`
	Memory resource.Quantity `json:"memory"`
}

type NervexJobResponse struct {
	Namespace   string   `json:"namespace"`
	Coordinator string   `json:"coordinator"`
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

	log.Info("Start listening on :8080")
	http.ListenAndServe(":8080", nil)
	return nil
}

func (s *NervexServer) Add(w http.ResponseWriter, r *http.Request) {
	log := s.Log.WithName("NernexServer")

	// parse request body
	var njreq NervexJobRequest
	err := json.NewDecoder(r.Body).Decode(&njreq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("request from", njreq.Namespace, njreq.Coordinator)

	// get ALConfig
	obj, exists, err := s.alconfigDyInformer.GetIndexer().GetByKey(s.alconfig)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		if ref.Kind == "NervexJob" {
			ownRefer = ref
		}
	}

	// get NervexJob
	njKey := fmt.Sprintf("%s/%s", njreq.Namespace, ownRefer.Name)
	obj, exists, err = s.njDyInformer.GetIndexer().GetByKey(njKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !exists {
		http.Error(w, fmt.Sprintf("NervexJob: %s not exists", njKey), http.StatusInternalServerError)
		return
	}

	nj := &nervexv1alpha1.NervexJob{}
	err = nervexutil.GetObjectFromUnstructured(obj, nj)

	// create actors and learners
	actors, learners, err := s.createActorsAndLearnersFromALConfig(alconfig, &njreq, ownRefer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(repJson)
}

func (s *NervexServer) createActorsAndLearnersFromALConfig(
	alconfig *nervexv1alpha1.ActorLearnerConfig,
	njreq *NervexJobRequest,
	ownRefer metav1.OwnerReference) ([]string, []string, error) {

	// create actors
	actorTemplate := alconfig.Spec.Actor.Template
	actors, err := s.createPodsAndServices(&actorTemplate, ownRefer, njreq.Namespace, njreq.Coordinator, njreq.Actors.Number,
		nervexutil.ActorName, nervexutil.DefaultActorContainerName, nervexutil.DefaultActorPortName, nervexutil.DefaultActorPort)

	if err != nil {
		return nil, nil, err
	}

	// create learners
	learnerTemplate := alconfig.Spec.Learner.Template
	learners, err := s.createPodsAndServices(&learnerTemplate, ownRefer, njreq.Namespace, njreq.Coordinator, njreq.Learners.Number,
		nervexutil.LearnerName, nervexutil.DefaultLearnerContainerName, nervexutil.DefaultLearnerPortName, nervexutil.DefaultLearnerPort)

	if err != nil {
		return nil, nil, err
	}

	return actors, learners, nil
}

func (s *NervexServer) createPodsAndServices(
	template *corev1.PodTemplateSpec,
	ownRefer metav1.OwnerReference,
	ns, coordinator string,
	replicas int,
	replicaType, containerName, portName string, defaultPort int32) ([]string, error) {

	// add labels to pod template
	labels := nervexutil.GenLabels(ownRefer.Name)
	labels[nervexutil.ReplicaTypeLabel] = replicaType
	nervexutil.AddLabelsToPodTemplate(template, labels)

	// generate pod from template
	pod := nervexutil.BuildPodFromTemplate(template.DeepCopy())
	pod.GenerateName = fmt.Sprintf("%s-%s-", coordinator, replicaType)
	pod.Namespace = ns
	pod.SetOwnerReferences([]metav1.OwnerReference{ownRefer})

	// get pod port
	port, ok := nervexutil.GetPortFromPod(pod, containerName, portName)
	if !ok {
		port = defaultPort
	}

	// build service
	svc := corev1.Service{
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Selector:  labels,
			Ports: []corev1.ServicePort{
				{
					Port: port,
					Name: portName,
				},
			},
		},
	}
	svc.Labels = labels
	svc.SetOwnerReferences([]metav1.OwnerReference{ownRefer})

	results := []string{}
	// create pods and services
	for i := 0; i < replicas; i++ {
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

func (s *NervexServer) Delete(w http.ResponseWriter, r *http.Request) {

}
