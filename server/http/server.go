package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
	nervexutil "go-sensephoenix.sensetime.com/nervex-operator/utils"
)

var (
	addReplicas        = "/api/v1alpha1/addReplicas"
	deleteReplicas     = "/api/v1alpha1/deleteReplicas"
	userAddReplicas    = "/api/v1alpha1/userAddReplicas"
	userDeleteReplicas = "/api/v1alpha1/userDeleteReplicas"
)

func (s *NerveXServer) Start(serverBindAddress string) error {
	log := s.Log.WithName("NerveXServer")
	http.HandleFunc(addReplicas, s.AddReplicas)
	http.HandleFunc(deleteReplicas, s.DeleteReplicas)
	http.HandleFunc("/healthz", healthz)

	log.Info("Start listening on", "port", serverBindAddress)
	if err := http.ListenAndServe(serverBindAddress, nil); err != nil {
		return err
	}
	return nil
}

func (s *NerveXServer) AddReplicas(w http.ResponseWriter, r *http.Request) {
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
		log.Error(err, "failed to get OwnerReference of coordinator", "coordinator", nervexutil.NamespacedName(njreq.Namespace, njreq.Coordinator))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// create actors and learners
	actors, learners, err := s.createActorsAndLearnersFromALConfig(alconfig, &njreq, *ownRefer)
	if err != nil {
		errMsg := fmt.Sprintf("failed create actors and learners: %s", err)
		log.Error(err, "failed create actors and learners")
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	// write response
	writeResponse(w, njreq.Namespace, njreq.Coordinator, actors, learners)
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
	_, exists, err := s.njDyi.GetIndexer().GetByKey(njKey)
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

func writeResponse(w http.ResponseWriter, ns, coordinator string, actors, learners []string) {
	rep := NerveXJobResponse{
		Namespace:   ns,
		Coordinator: coordinator,
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

func (s *NerveXServer) getALConfig() (*nervexv1alpha1.ActorLearnerConfig, error) {
	obj, exists, err := s.alDyi.GetIndexer().GetByKey(s.alconfig)
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

func (s *NerveXServer) listReplicaPods(ns, jobName string) (
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

func (s *NerveXServer) createPodsAndServices(
	template *corev1.PodTemplateSpec,
	ownRefer metav1.OwnerReference,
	njreq *NerveXJobRequest,
	replicaType, containerName, portName string, defaultPort int32) ([]string, error) {

	ns := njreq.Namespace
	resources := ResourceQuantity{}
	switch replicaType {
	case nervexutil.ActorName:
		resources = njreq.Actors
	case nervexutil.LearnerName:
		resources = njreq.Learners
	case nervexutil.AggregatorName:
		resources = ResourceQuantity{Replicas: 1}
	}

	results := []string{}
	// create pods and services
	for i := 0; i < resources.Replicas; i++ {
		// build pod
		pod, port, err := nervexutil.BuildPodFromTemplate(template.DeepCopy(), ownRefer, ns, replicaType, containerName, portName, defaultPort)
		if err != nil {
			return nil, err
		}
		// set pod resources
		SetPodResources(pod, resources, containerName)

		// create pod
		_, err = s.KubeClient.CoreV1().Pods(ns).Create(context.Background(), pod, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}

		// build service
		svc := nervexutil.BuildService(pod.GetLabels(), port, portName)
		svc.SetOwnerReferences([]metav1.OwnerReference{ownRefer})
		svc.Name = pod.Name

		// create service
		_, err = s.KubeClient.CoreV1().Services(ns).Create(context.Background(), svc, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}

		result := fmt.Sprintf("%s.%s:%d", svc.Name, ns, port)
		results = append(results, result)
	}

	return results, nil
}

func (s *NerveXServer) DeleteReplicas(w http.ResponseWriter, r *http.Request) {
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
	actors, learners, _, _, err := s.listReplicaPods(njreq.Namespace, ownRefer.Name)
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
	writeResponse(w, njreq.Namespace, njreq.Coordinator, delActors, delLearners)
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
