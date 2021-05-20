package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
	serverdynamic "go-sensephoenix.sensetime.com/nervex-operator/server/dynamic"
	servertypes "go-sensephoenix.sensetime.com/nervex-operator/server/types"
	nervexutil "go-sensephoenix.sensetime.com/nervex-operator/utils"
)

var (
	replicasAPI       = "/v1alpha1/replicas"
	replicasFailedAPI = "/v1alpha1/replicas/failed"
)

type NerveXServer struct {
	KubeClient    *kubernetes.Clientset
	DynamicClient dynamic.Interface
	Log           logr.Logger
	dyi           serverdynamic.Informers
}

func NewNerveXServer(
	kubeClient *kubernetes.Clientset,
	dynamicClient dynamic.Interface,
	log logr.Logger,
	dyi serverdynamic.Informers) *NerveXServer {

	return &NerveXServer{
		KubeClient:    kubeClient,
		DynamicClient: dynamicClient,
		Log:           log,
		dyi:           dyi,
	}
}

func (s *NerveXServer) Start(serverBindAddress string) error {
	log := s.Log.WithName("NerveXServer")
	http.HandleFunc(replicasAPI, s.Replicas)
	http.HandleFunc(replicasFailedAPI, s.ReplicasFailed)
	http.HandleFunc("/healthz", healthz)

	log.Info("Start listening on", "port", serverBindAddress)
	if err := http.ListenAndServe(serverBindAddress, nil); err != nil {
		return err
	}
	return nil
}

func (s *NerveXServer) Replicas(w http.ResponseWriter, r *http.Request) {
	log := s.Log.WithName("NerveXServer")

	var reps interface{}
	var err error
	var msg string

	// handle request by request method
	switch r.Method {
	case "GET":
		msg = "successfully get replicas"
		reps, err = s.getReplicas(r)
	case "POST":
		msg = "successfully create replicas"
		reps, err = s.addReplicas(r)
	case "DELETE":
		msg = "successfully delete replicas"
		reps, err = s.deleteReplicas(r)
	default:
		err = &servertypes.NerveXError{Type: servertypes.ErrorNotImplemented, Message: fmt.Sprintf("%s not implemented", r.Method)}
		log.Error(err, "method not implemented")
	}

	rep, statusCode := s.buildResponse(reps, msg, err)

	// write response
	if err = writeResponse(w, rep, statusCode); err != nil {
		log.Error(err, "failed to write response")
	}
}

func (s *NerveXServer) getReplicas(r *http.Request) (interface{}, error) {
	// get request params from request
	rp := servertypes.NerveXJobRequestParams{}
	params := r.URL.Query()
	for k, v := range params {
		switch strings.ToLower(k) {
		case servertypes.RequestParamTypeNamespace:
			rp.Namespace = v
		case servertypes.RequestParamTypeCoordinator:
			rp.Coordinator = v
		case servertypes.RequestParamTypeName:
			rp.Name = v
		default:
			errInfo := fmt.Sprintf("request param %s is not supported", k)
			return nil, &servertypes.NerveXError{Type: servertypes.ErrorBadRequest, Message: errInfo}
		}
	}

	var reps interface{}
	var err error
	if rp.Namespace == nil { // if namespace not set, get all replicas
		reps, err = s.getAllReplicas()
		if err != nil {
			return nil, err
		}
	} else if rp.Coordinator == nil && rp.Name == nil { //
		reps, err = s.getNamespacedReplicas(rp.Namespace[0])
		if err != nil {
			return nil, err
		}
	} else if rp.Name == nil {
		reps, err = s.getNamespacedReplicasByCoordinator(rp.Namespace[0], rp.Coordinator[0])
		if err != nil {
			return nil, err
		}
	} else if rp.Coordinator == nil {
		s.getNamespacedReplicaByName(rp.Namespace[0], rp.Name[0])
	} else {
		s.getNamespacedReplicasByCoordinatorAndName(rp.Namespace[0], rp.Coordinator[0], rp.Name[0])
	}

	return reps, nil
}

func (s *NerveXServer) getAllReplicas() ([]servertypes.NerveXJobResponse, error) {
	nsl, err := s.KubeClient.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	results := []servertypes.NerveXJobResponse{}
	for _, ns := range nsl.Items {
		reps, err := s.getNamespacedReplicas(ns.Name)
		if err != nil {
			return nil, err
		}
		results = append(results, reps...)
	}
	return results, nil
}

func (s *NerveXServer) getNamespacedReplicas(namespace string) ([]servertypes.NerveXJobResponse, error) {
	log := s.Log.WithName("NerveXServer")

	// construct label selector to list coordinators in namespace
	lbs := map[string]string{
		nervexutil.GroupNameLabel:      nervexv1alpha1.GroupVersion.Group,
		nervexutil.ControllerNameLabel: nervexutil.ControllerName,
		nervexutil.ReplicaTypeLabel:    nervexutil.CoordinatorName,
	}
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: lbs,
	})
	if err != nil {
		return nil, err
	}

	// list coordinators in namespace
	pods, err := s.ListPodsWithSelector(namespace, labelSelector)
	if err != nil {
		return nil, err
	}

	results := []servertypes.NerveXJobResponse{}
	for _, pod := range pods {
		result, err := s.getNamespacedReplicasByCoordinator(namespace, pod.Name)
		if err != nil {
			errMsg := fmt.Sprintf("failed to get replicas for coordinator %s, skipped", pod.Name)
			log.Error(err, errMsg)
		}
		results = append(results, result)
	}

	return results, nil
}

func (s *NerveXServer) getNamespacedReplicasByCoordinator(namespace, coordinatorName string) (servertypes.NerveXJobResponse, error) {
	log := s.Log.WithName("NerveXServer")

	// get ownReference of the request coordinator
	nvxJob, err := s.GetNerveXJob(namespace, coordinatorName)
	if err != nil {
		log.Error(err, "failed to get owner reference")
		return servertypes.NerveXJobResponse{}, err
	}

	// list pods that belong to the NerveXJob
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: nervexutil.GenLabels(nvxJob.Name),
	})
	if err != nil {
		return servertypes.NerveXJobResponse{}, err
	}
	collectors, learners, _, _, err := s.ListReplicaPodsWithSelector(namespace, labelSelector)
	if err != nil {
		log.Error(err, "failed to list collectors and learners")
		return servertypes.NerveXJobResponse{}, err
	}

	// get access urls
	collectorURLs := []string{}
	learnerURLs := []string{}
	for _, pod := range collectors {
		url := nervexutil.GetPodAccessURL(pod, namespace,
			nervexutil.DefaultCollectorContainerName, nervexutil.DefaultCollectorPortName, nervexutil.DefaultCollectorPort)
		collectorURLs = append(collectorURLs, url)
	}
	for _, pod := range learners {
		url := nervexutil.GetPodAccessURL(pod, namespace,
			nervexutil.DefaultLearnerContainerName, nervexutil.DefaultLearnerPortName, nervexutil.DefaultLearnerPort)
		learnerURLs = append(learnerURLs, url)
	}

	rep := servertypes.NerveXJobResponse{
		Namespace:   namespace,
		Coordinator: coordinatorName,
		Collectors:  collectorURLs,
		Learners:    learnerURLs,
	}

	return rep, nil
}

func (s *NerveXServer) getNamespacedReplicaByName(namespace, name string) {

}

func (s *NerveXServer) getNamespacedReplicasByCoordinatorAndName(namespace, coordinatorName, name string) {

}

// add replicas api
func (s *NerveXServer) addReplicas(r *http.Request) (servertypes.NerveXJobResponse, error) {
	log := s.Log.WithName("NerveXServer")
	// get request body
	var njreq servertypes.NerveXJobRequest
	err := json.NewDecoder(r.Body).Decode(&njreq)
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode request body: %v", err)
		return servertypes.NerveXJobResponse{}, &servertypes.NerveXError{Type: servertypes.ErrorBadRequest, Message: errMsg}
	}

	// get ownReference of request coordinator
	nvxJob, err := s.GetNerveXJob(njreq.Namespace, njreq.Coordinator)
	if err != nil {
		return servertypes.NerveXJobResponse{}, err
	}

	// create collectors and learners
	collectors, learners, err := s.CreateCollectorsAndLearnersForNerveXJob(&njreq, nvxJob)
	if err != nil {
		return servertypes.NerveXJobResponse{}, err
	}
	log.Info("create replicas", "collectors", collectors, "learners", learners)

	rep := servertypes.NerveXJobResponse{
		Namespace:   njreq.Namespace,
		Coordinator: njreq.Coordinator,
		Collectors:  collectors,
		Learners:    learners,
	}

	return rep, nil
}

// delete replicas api
func (s *NerveXServer) deleteReplicas(r *http.Request) (servertypes.NerveXJobResponse, error) {
	log := s.Log.WithName("NerveXServer")
	// get request body
	var njreq servertypes.NerveXJobRequest
	err := json.NewDecoder(r.Body).Decode(&njreq)
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode request body: %v", err)
		return servertypes.NerveXJobResponse{}, &servertypes.NerveXError{Type: servertypes.ErrorBadRequest, Message: errMsg}
	}

	// get ownReference of the request coordinator
	nvxJob, err := s.GetNerveXJob(njreq.Namespace, njreq.Coordinator)
	if err != nil {
		return servertypes.NerveXJobResponse{}, err
	}

	// list pods that belong to the NerveXJob
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: nervexutil.GenLabels(nvxJob.Name),
	})
	if err != nil {
		return servertypes.NerveXJobResponse{}, err
	}
	collectors, learners, _, _, err := s.ListReplicaPodsWithSelector(njreq.Namespace, labelSelector)
	if err != nil {
		return servertypes.NerveXJobResponse{}, err
	}

	// delete collector pods
	delCollectors, err := s.DeleteReplicas(collectors, njreq.Namespace, njreq.Collectors.Replicas, nervexutil.CollectorName)
	if err != nil {
		return servertypes.NerveXJobResponse{}, err
	}

	// delete learner pods
	delLearners, err := s.DeleteReplicas(learners, njreq.Namespace, njreq.Learners.Replicas, nervexutil.LearnerName)
	if err != nil {
		return servertypes.NerveXJobResponse{}, err
	}

	log.Info("delete replicas", "collectors", delCollectors, "learners", delLearners)

	rep := servertypes.NerveXJobResponse{
		Namespace:   njreq.Namespace,
		Coordinator: njreq.Coordinator,
		Collectors:  delCollectors,
		Learners:    delLearners,
	}

	return rep, nil
}

// ReplicasFailed will delete the failed replicas reported by caller, and recreate the same number of replicas
func (s *NerveXServer) ReplicasFailed(w http.ResponseWriter, r *http.Request) {
	log := s.Log.WithName("NerveXServer")

	var reps interface{}
	var err error
	var msg string
	switch r.Method {
	case "POST":
		msg = "successfully recreate replicas"
		reps, err = s.replicasFailed(r)
	default:
		err = &servertypes.NerveXError{Type: servertypes.ErrorNotImplemented, Message: fmt.Sprintf("%s not implemented", r.Method)}
		log.Error(err, "method not implemented")
	}

	rep, statusCode := s.buildResponse(reps, msg, err)
	// write response
	if err = writeResponse(w, rep, statusCode); err != nil {
		log.Error(err, "failed to write response")
	}
}

func (s *NerveXServer) replicasFailed(r *http.Request) (servertypes.NerveXJobResponse, error) {
	log := s.Log.WithName("NerveXServer")

	// parse request body
	var njreq servertypes.NerveXJobResponse
	err := json.NewDecoder(r.Body).Decode(&njreq)
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode request body: %v", err)
		return servertypes.NerveXJobResponse{}, &servertypes.NerveXError{Type: servertypes.ErrorBadRequest, Message: errMsg}
	}
	log.Info("delete request body: ", "request", njreq)

	cpods, err := s.GetPodsByNames(njreq.Namespace, njreq.Collectors)
	collectors, err := s.RecreateReplicas(cpods, njreq.Namespace, nervexutil.CollectorName)
	if err != nil {
		return servertypes.NerveXJobResponse{}, err
	}
	lpods, err := s.GetPodsByNames(njreq.Namespace, njreq.Learners)
	learners, err := s.RecreateReplicas(lpods, njreq.Namespace, nervexutil.LearnerName)
	if err != nil {
		return servertypes.NerveXJobResponse{}, err
	}

	rep := servertypes.NerveXJobResponse{
		Namespace:   njreq.Namespace,
		Coordinator: njreq.Coordinator,
		Collectors:  collectors,
		Learners:    learners,
	}
	return rep, nil
}

func (s *NerveXServer) buildResponse(reps interface{}, msg string, err error) (servertypes.Response, int) {
	log := s.Log.WithName("NerveXServer")

	var success bool = true
	var code int = servertypes.CodeSuccess
	var statusCode int = http.StatusOK
	if err != nil {
		success = false
		code = servertypes.CodeFailed
		msg = err.Error()

		// define status code
		if servertypes.IsNotFound(err) {
			statusCode = http.StatusNotFound
		} else if servertypes.IsAlreadyExists(err) {
			statusCode = http.StatusConflict
		} else if servertypes.IsBadRequest(err) {
			statusCode = http.StatusBadRequest
		} else if servertypes.IsNotImplemented(err) {
			statusCode = http.StatusNotImplemented
		} else {
			statusCode = http.StatusInternalServerError
		}

		log.Error(err, "failed to process request")
	}

	// build response
	rep := servertypes.Response{
		Success: success,
		Code:    code,
		Message: msg,
		Data:    reps,
	}
	return rep, statusCode
}

func writeResponse(w http.ResponseWriter, rep servertypes.Response, statusCode int) error {
	w.Header().Set("Conten-Type", "application/json")
	w.WriteHeader(statusCode)
	repJSON, err := json.Marshal(rep)
	if err != nil {
		errMsg := fmt.Sprintf("failed to marshal json: %s", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return err
	}
	_, err = w.Write(repJSON)
	if err != nil {
		errMsg := fmt.Sprintf("failed to write json: %s", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return err
	}
	return nil
}