package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	div1alpha1 "go-sensephoenix.sensetime.com/di-orchestrator/api/v1alpha1"
	dicommon "go-sensephoenix.sensetime.com/di-orchestrator/common"
	serverdynamic "go-sensephoenix.sensetime.com/di-orchestrator/server/dynamic"
	servertypes "go-sensephoenix.sensetime.com/di-orchestrator/server/types"
	diutil "go-sensephoenix.sensetime.com/di-orchestrator/utils"
)

var (
	apiVersion        = "v1alpha1"
	replicasAPI       = "/replicas"
	replicasFailedAPI = "/replicas/failed"
)

func withAPIVersion(api string) string {
	return fmt.Sprintf("/%s%s", apiVersion, api)
}

type DIServer struct {
	KubeClient    *kubernetes.Clientset
	DynamicClient dynamic.Interface
	Log           logr.Logger
	AGConfig      string
	dyi           serverdynamic.Informers
	gpuAllocator  dicommon.GPUAllocator
}

func NewDIServer(
	kubeClient *kubernetes.Clientset,
	dynamicClient dynamic.Interface,
	log logr.Logger,
	agconfig string,
	dyi serverdynamic.Informers,
	gpuAllocPolicy string) *DIServer {

	var gpuAllocator dicommon.GPUAllocator
	switch gpuAllocPolicy {
	case dicommon.SimpleGPUAllocPolicy:
		gpuAllocator = *dicommon.NewSimpleGPUAllocator([]*corev1.Node{})
	}
	return &DIServer{
		KubeClient:    kubeClient,
		DynamicClient: dynamicClient,
		Log:           log,
		AGConfig:      agconfig,
		dyi:           dyi,
		gpuAllocator:  gpuAllocator,
	}
}

func (s *DIServer) Start(serverBindAddress string) error {
	log := s.Log.WithName("DIServer")
	http.HandleFunc(withAPIVersion(replicasAPI), s.Replicas)
	http.HandleFunc(withAPIVersion(replicasFailedAPI), s.ReplicasFailed)
	http.HandleFunc("/healthz", healthz)

	log.Info("Start listening on", "port", serverBindAddress)
	if err := http.ListenAndServe(serverBindAddress, nil); err != nil {
		return err
	}
	return nil
}

func (s *DIServer) SyncNodes() error {
	rets, err := s.dyi.NodeInformer.Lister().List(labels.Everything())
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	var nodes []*corev1.Node
	for _, ret := range rets {
		un := ret.(*unstructured.Unstructured)
		var node corev1.Node
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(un.UnstructuredContent(), &node); err != nil {
			return err
		}
		nodes = append(nodes, &node)
	}
	s.gpuAllocator.Nodes = nodes
	return nil
}

func (s *DIServer) Replicas(w http.ResponseWriter, r *http.Request) {
	log := s.Log.WithName("DIServer")

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
		err = &servertypes.DIError{Type: servertypes.ErrorNotImplemented, Message: fmt.Sprintf("%s not implemented", r.Method)}
		log.Error(err, "method not implemented")
	}

	rep, statusCode := s.buildResponse(reps, msg, err)

	// write response
	if err = writeResponse(w, rep, statusCode); err != nil {
		log.Error(err, "failed to write response")
	}
}

func (s *DIServer) getReplicas(r *http.Request) (interface{}, error) {
	// get request params from request
	rp := servertypes.DIJobRequestParams{}
	params := r.URL.Query()
	for k, v := range params {
		switch strings.ToLower(k) {
		case servertypes.RequestParamTypeNamespace:
			rp.Namespace = v
		case servertypes.RequestParamTypeCoordinator:
			rp.Coordinator = v
		case servertypes.RequestParamTypeName:
			rp.Name = v
		case servertypes.RequestParamTypeAggregator:
			rp.Aggregator = v
		default:
			errInfo := fmt.Sprintf("request param %s is not supported", k)
			return nil, &servertypes.DIError{Type: servertypes.ErrorBadRequest, Message: errInfo}
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
		if rp.Aggregator != nil {
			reps, err = s.getNamespacedDDPLearnersByAggregator(rp.Namespace[0], rp.Aggregator[0])
			if err != nil {
				return nil, err
			}
		} else {
			reps, err = s.getNamespacedReplicas(rp.Namespace[0])
			if err != nil {
				return nil, err
			}
		}
	} else if rp.Name == nil {
		reps, err = s.getNamespacedReplicasByCoordinator(rp.Namespace[0], rp.Coordinator[0])
		if err != nil {
			return nil, err
		}
	}

	return reps, nil
}

func (s *DIServer) getAllReplicas() ([]servertypes.DIJobResponse, error) {
	nsl, err := s.KubeClient.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	results := []servertypes.DIJobResponse{}
	for _, ns := range nsl.Items {
		reps, err := s.getNamespacedReplicas(ns.Name)
		if err != nil {
			return nil, err
		}
		results = append(results, reps...)
	}
	return results, nil
}

func (s *DIServer) getNamespacedReplicas(namespace string) ([]servertypes.DIJobResponse, error) {
	log := s.Log.WithName("DIServer")

	// construct label selector to list coordinators in namespace
	lbs := map[string]string{
		diutil.GroupNameLabel:      div1alpha1.GroupVersion.Group,
		diutil.ControllerNameLabel: diutil.ControllerName,
		diutil.ReplicaTypeLabel:    diutil.CoordinatorName,
	}
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: lbs,
	})
	if err != nil {
		return nil, err
	}

	// list coordinators in namespace
	pods, err := s.listPodsWithSelector(namespace, labelSelector)
	if err != nil {
		return nil, err
	}

	results := []servertypes.DIJobResponse{}
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

func (s *DIServer) getNamespacedReplicasByCoordinator(namespace, coordinatorName string) (servertypes.DIJobResponse, error) {
	log := s.Log.WithName("DIServer")

	// get ownReference of the request coordinator
	diJob, err := s.getDIJob(namespace, coordinatorName)
	if err != nil {
		log.Error(err, "failed to get owner reference")
		return servertypes.DIJobResponse{}, err
	}

	// list pods that belong to the DIJob
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: diutil.GenLabels(diJob.Name),
	})
	if err != nil {
		return servertypes.DIJobResponse{}, err
	}
	collectors, learners, _, aggregators, _, err := s.listReplicaServicesWithSelector(namespace, labelSelector)
	if err != nil {
		log.Error(err, "failed to list collectors and learners")
		return servertypes.DIJobResponse{}, err
	}

	// get access urls
	collectorURLs := []string{}
	learnerURLs := []string{}
	for _, svc := range collectors {
		url := diutil.GetServiceAccessURL(svc)
		collectorURLs = append(collectorURLs, url)
	}
	for _, svc := range learners {
		url := diutil.GetServiceAccessURL(svc)
		learnerURLs = append(learnerURLs, url)
	}

	// aggregators are also considered to be learners in view of coordinator
	for _, svc := range aggregators {
		url := diutil.GetServiceAccessURL(svc)
		learnerURLs = append(learnerURLs, url)
	}

	rep := servertypes.DIJobResponse{
		Namespace:   namespace,
		Coordinator: coordinatorName,
		Collectors:  collectorURLs,
		Learners:    learnerURLs,
	}

	log.Info("get replicas", "collectors", collectorURLs, "learners", learnerURLs)
	return rep, nil
}

func (s *DIServer) getNamespacedDDPLearnersByAggregator(namespace, aggregatorName string) (servertypes.DIJobResponse, error) {
	log := s.Log.WithName("DIServer")

	// get ownReference of the request coordinator
	diJob, err := s.getDIJob(namespace, aggregatorName)
	if err != nil {
		log.Error(err, "failed to get owner reference")
		return servertypes.DIJobResponse{}, err
	}

	// list pods that belong to the DIJob
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: diutil.GenLabels(diJob.Name),
	})
	if err != nil {
		return servertypes.DIJobResponse{}, err
	}
	_, _, _, _, DDPLearners, err := s.listReplicaServicesWithSelector(namespace, labelSelector)
	if err != nil {
		log.Error(err, "failed to list collectors and learners")
		return servertypes.DIJobResponse{}, err
	}

	// get access urls
	ddpLearnerURLs := []string{}
	for _, svc := range DDPLearners {
		owners := svc.GetOwnerReferences()
		owns := false
		for _, owner := range owners {
			if owner.Name == aggregatorName {
				owns = true
				break
			}
		}
		if !owns {
			continue
		}

		// build access urls to ddp learners
		url := diutil.GetServiceAccessURL(svc)
		ddpLearnerURLs = append(ddpLearnerURLs, url)

		// append all gpu process access urls to response
		for _, port := range svc.Spec.Ports {
			if !strings.HasPrefix(port.Name, diutil.DDPLearnerPortPrefix) {
				continue
			}
			url := diutil.ConcatURL(svc.Name, namespace, port.Port)
			ddpLearnerURLs = append(ddpLearnerURLs, url)
		}
	}

	rep := servertypes.DIJobResponse{
		Namespace:   namespace,
		Coordinator: aggregatorName,
		Learners:    ddpLearnerURLs,
	}
	return rep, nil
}

// add replicas api
func (s *DIServer) addReplicas(r *http.Request) (servertypes.DIJobResponse, error) {
	log := s.Log.WithName("DIServer")
	// get request body
	var njreq servertypes.DIJobRequest
	err := json.NewDecoder(r.Body).Decode(&njreq)
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode request body: %v", err)
		return servertypes.DIJobResponse{}, &servertypes.DIError{Type: servertypes.ErrorBadRequest, Message: errMsg}
	}

	// get ownReference of request coordinator
	diJob, err := s.getDIJob(njreq.Namespace, njreq.Coordinator)
	if err != nil {
		return servertypes.DIJobResponse{}, err
	}

	// create collectors and learners
	collectors, learners, err := s.createCollectorsAndLearnersForDIJob(&njreq, diJob)
	if err != nil {
		return servertypes.DIJobResponse{
			Namespace:   njreq.Namespace,
			Coordinator: njreq.Coordinator,
			Collectors:  collectors,
			Learners:    learners,
		}, err
	}
	log.Info("create replicas", "collectors", collectors, "learners", learners)

	rep := servertypes.DIJobResponse{
		Namespace:   njreq.Namespace,
		Coordinator: njreq.Coordinator,
		Collectors:  collectors,
		Learners:    learners,
	}

	return rep, nil
}

// delete replicas api
func (s *DIServer) deleteReplicas(r *http.Request) (servertypes.DIJobResponse, error) {
	log := s.Log.WithName("DIServer")
	// get request body
	var njreq servertypes.DIJobRequest
	err := json.NewDecoder(r.Body).Decode(&njreq)
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode request body: %v", err)
		return servertypes.DIJobResponse{}, &servertypes.DIError{Type: servertypes.ErrorBadRequest, Message: errMsg}
	}

	// get ownReference of the request coordinator
	diJob, err := s.getDIJob(njreq.Namespace, njreq.Coordinator)
	if err != nil {
		return servertypes.DIJobResponse{}, err
	}

	// list pods that belong to the DIJob
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: diutil.GenLabels(diJob.Name),
	})
	if err != nil {
		return servertypes.DIJobResponse{}, err
	}
	collectors, learners, _, aggs, _, err := s.listReplicaPodsWithSelector(njreq.Namespace, labelSelector)
	if err != nil {
		return servertypes.DIJobResponse{}, err
	}

	// delete collector pods
	delCollectors, err := s.deleteSpecifiedReplicas(collectors, njreq.Namespace, njreq.Collectors.Replicas, diutil.CollectorName)
	if err != nil {
		return servertypes.DIJobResponse{
			Namespace:   njreq.Namespace,
			Coordinator: njreq.Coordinator,
			Collectors:  delCollectors,
			Learners:    nil,
		}, err
	}

	// delete learner pods
	delLearners, err := s.deleteSpecifiedReplicas(learners, njreq.Namespace, njreq.Learners.Replicas, diutil.LearnerName)
	if err != nil {
		return servertypes.DIJobResponse{
			Namespace:   njreq.Namespace,
			Coordinator: njreq.Coordinator,
			Collectors:  delCollectors,
			Learners:    delLearners,
		}, err
	}

	// aggregator is also considered a learner
	if len(delLearners) <= 0 {
		delAggs, err := s.deleteSpecifiedReplicas(aggs, njreq.Namespace, njreq.Learners.Replicas, diutil.AggregatorName)
		if err != nil {
			return servertypes.DIJobResponse{
				Namespace:   njreq.Namespace,
				Coordinator: njreq.Coordinator,
				Collectors:  delCollectors,
				Learners:    delLearners,
			}, err
		}
		delLearners = append(delLearners, delAggs...)
	}

	log.Info("delete replicas", "collectors", delCollectors, "learners", delLearners)

	rep := servertypes.DIJobResponse{
		Namespace:   njreq.Namespace,
		Coordinator: njreq.Coordinator,
		Collectors:  delCollectors,
		Learners:    delLearners,
	}

	return rep, nil
}

// ReplicasFailed will delete the failed replicas reported by caller, and recreate the same number of replicas
func (s *DIServer) ReplicasFailed(w http.ResponseWriter, r *http.Request) {
	log := s.Log.WithName("DIServer")

	var reps interface{}
	var err error
	var msg string
	switch r.Method {
	case "POST":
		msg = "successfully recreate replicas"
		reps, err = s.replicasFailed(r)
	default:
		err = &servertypes.DIError{Type: servertypes.ErrorNotImplemented, Message: fmt.Sprintf("%s not implemented", r.Method)}
		log.Error(err, "method not implemented")
	}

	rep, statusCode := s.buildResponse(reps, msg, err)
	// write response
	if err = writeResponse(w, rep, statusCode); err != nil {
		log.Error(err, "failed to write response")
	}
}

func (s *DIServer) replicasFailed(r *http.Request) (servertypes.DIJobResponse, error) {
	log := s.Log.WithName("DIServer")

	// parse request body
	var njreq servertypes.DIJobResponse
	err := json.NewDecoder(r.Body).Decode(&njreq)
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode request body: %v", err)
		return servertypes.DIJobResponse{}, &servertypes.DIError{Type: servertypes.ErrorBadRequest, Message: errMsg}
	}
	log.Info("failed replicas request body: ", "request", njreq)

	// get collector pods and services
	cpods, err := s.getPodsByNames(njreq.Namespace, njreq.Collectors)
	if err != nil {
		return servertypes.DIJobResponse{}, err
	}
	csvcs, err := s.getServicesByNames(njreq.Namespace, njreq.Collectors)
	if err != nil {
		return servertypes.DIJobResponse{}, err
	}

	collectors, err := s.recreateReplicas(cpods, csvcs, njreq.Namespace)
	if err != nil {
		return servertypes.DIJobResponse{
			Namespace:   njreq.Namespace,
			Coordinator: njreq.Coordinator,
			Collectors:  collectors,
			Learners:    nil,
		}, err
	}

	lpods, err := s.getPodsByNames(njreq.Namespace, njreq.Learners)
	if err != nil {
		return servertypes.DIJobResponse{
			Namespace:   njreq.Namespace,
			Coordinator: njreq.Coordinator,
			Collectors:  collectors,
			Learners:    nil,
		}, err
	}
	lsvcs, err := s.getServicesByNames(njreq.Namespace, njreq.Learners)
	if err != nil {
		return servertypes.DIJobResponse{
			Namespace:   njreq.Namespace,
			Coordinator: njreq.Coordinator,
			Collectors:  collectors,
			Learners:    nil,
		}, err
	}

	learners, err := s.recreateReplicas(lpods, lsvcs, njreq.Namespace)
	if err != nil {
		return servertypes.DIJobResponse{
			Namespace:   njreq.Namespace,
			Coordinator: njreq.Coordinator,
			Collectors:  collectors,
			Learners:    learners,
		}, err
	}

	log.Info("recreate replicas", "collectors", collectors, "learners", learners)

	rep := servertypes.DIJobResponse{
		Namespace:   njreq.Namespace,
		Coordinator: njreq.Coordinator,
		Collectors:  collectors,
		Learners:    learners,
	}
	return rep, nil
}

func (s *DIServer) buildResponse(reps interface{}, msg string, err error) (servertypes.Response, int) {
	log := s.Log.WithName("DIServer")

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
