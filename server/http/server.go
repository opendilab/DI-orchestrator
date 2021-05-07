package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
	. "go-sensephoenix.sensetime.com/nervex-operator/server/errors"
	nervexutil "go-sensephoenix.sensetime.com/nervex-operator/utils"
)

var (
	replicasAPI       = "/v1alpha1/replicas"
	replicasFailedAPI = "/v1alpha1/replicas/failed"
)

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
	}

	var success bool = true
	var code int = CodeSuccess
	var statusCode int = http.StatusOK
	if err != nil {
		success = false
		code = CodeFailed
		msg = err.Error()

		// define status code
		if IsNotFound(err) {
			statusCode = http.StatusNotFound
		} else if IsAlreadyExists(err) {
			statusCode = http.StatusConflict
		} else if IsBadRequest(err) {
			statusCode = http.StatusBadRequest
		} else {
			statusCode = http.StatusInternalServerError
		}

		log.Error(err, "failed to process request")
	}

	// build response
	rep := Response{
		Success: success,
		Code:    code,
		Message: msg,
		Data:    reps,
	}

	// write response
	if err = writeResponse(w, rep, statusCode); err != nil {
		log.Error(err, "failed to write response")
	}
}

func (s *NerveXServer) getReplicas(r *http.Request) (interface{}, error) {
	// get request params from request
	rp := NerveXJobRequestParams{}
	params := r.URL.Query()
	for k, v := range params {
		switch strings.ToLower(k) {
		case RequestParamTypeNamespace:
			rp.Namespace = v
		case RequestParamTypeCoordinator:
			rp.Coordinator = v
		case RequestParamTypeName:
			rp.Name = v
		default:
			errInfo := fmt.Sprintf("request param %s is not supported", k)
			return nil, &NerveXError{Type: ErrorBadRequest, Message: errInfo}
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

func (s *NerveXServer) getAllReplicas() ([]NerveXJobResponse, error) {
	nsl, err := s.KubeClient.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	results := []NerveXJobResponse{}
	for _, ns := range nsl.Items {
		reps, err := s.getNamespacedReplicas(ns.Name)
		if err != nil {
			return nil, err
		}
		results = append(results, reps...)
	}
	return results, nil
}

func (s *NerveXServer) getNamespacedReplicas(namespace string) ([]NerveXJobResponse, error) {
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
	pods, err := s.listPodsWithSelector(namespace, labelSelector)
	if err != nil {
		return nil, err
	}

	results := []NerveXJobResponse{}
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

func (s *NerveXServer) getNamespacedReplicasByCoordinator(namespace, coordinatorName string) (NerveXJobResponse, error) {
	log := s.Log.WithName("NerveXServer")

	// get ownReference of the request coordinator
	nvxJob, err := s.getNerveXJob(namespace, coordinatorName)
	if err != nil {
		log.Error(err, "failed to get owner reference")
		return NerveXJobResponse{}, err
	}

	// list pods that belong to the NerveXJob
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: nervexutil.GenLabels(nvxJob.Name),
	})
	if err != nil {
		return NerveXJobResponse{}, err
	}
	collectors, learners, _, _, err := s.listReplicaPodsWithSelector(namespace, labelSelector)
	if err != nil {
		log.Error(err, "failed to list collectors and learners")
		return NerveXJobResponse{}, err
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

	rep := NerveXJobResponse{
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

func (s *NerveXServer) ReplicasFailed(w http.ResponseWriter, r *http.Request) {
	// log := s.Log.WithName("NerveXServer")

	// // parse request body
	// var njreq NerveXJobRequest
	// err := json.NewDecoder(r.Body).Decode(&njreq)
	// if err != nil {
	// 	http.Error(w, err.Error(), http.StatusBadRequest)
	// 	return
	// }
	// log.Info("delete request body: ", "request", njreq)

	// rep, err := s.deleteReplicas(njreq)
	// if err != nil {
	// 	log.Error(err, "failed to delete replicas")
	// 	http.Error(w, err.Error(), http.StatusInternalServerError)
	// }

	// // write response
	// if err = writeResponse(w, rep); err != nil {
	// 	log.Error(err, "failed to write response")
	// }
}

func (s *NerveXServer) addReplicas(r *http.Request) (NerveXJobResponse, error) {
	// get request body
	var njreq NerveXJobRequest
	err := json.NewDecoder(r.Body).Decode(&njreq)
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode request body: %v", err)
		return NerveXJobResponse{}, &NerveXError{Type: ErrorBadRequest, Message: errMsg}
	}

	// get ownReference of request coordinator
	nvxJob, err := s.getNerveXJob(njreq.Namespace, njreq.Coordinator)
	if err != nil {
		return NerveXJobResponse{}, err
	}

	// create collectors and learners
	collectors, learners, err := s.createCollectorsAndLearnersFromNerveXJob(&njreq, nvxJob)
	if err != nil {
		return NerveXJobResponse{}, err
	}

	rep := NerveXJobResponse{
		Namespace:   njreq.Namespace,
		Coordinator: njreq.Coordinator,
		Collectors:  collectors,
		Learners:    learners,
	}

	return rep, nil
}

func (s *NerveXServer) deleteReplicas(r *http.Request) (NerveXJobResponse, error) {
	// get request body
	var njreq NerveXJobRequest
	err := json.NewDecoder(r.Body).Decode(&njreq)
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode request body: %v", err)
		return NerveXJobResponse{}, &NerveXError{Type: ErrorBadRequest, Message: errMsg}
	}

	// get ownReference of the request coordinator
	nvxJob, err := s.getNerveXJob(njreq.Namespace, njreq.Coordinator)
	if err != nil {
		return NerveXJobResponse{}, err
	}

	// list pods that belong to the NerveXJob
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: nervexutil.GenLabels(nvxJob.Name),
	})
	if err != nil {
		return NerveXJobResponse{}, err
	}
	collectors, learners, _, _, err := s.listReplicaPodsWithSelector(njreq.Namespace, labelSelector)
	if err != nil {
		return NerveXJobResponse{}, err
	}

	// delete collector pods
	delCollectors, err := s.DeletePodsAndServices(collectors, &njreq, nervexutil.CollectorName)
	if err != nil {
		return NerveXJobResponse{}, err
	}

	// delete learner pods
	delLearners, err := s.DeletePodsAndServices(learners, &njreq, nervexutil.LearnerName)
	if err != nil {
		return NerveXJobResponse{}, err
	}

	rep := NerveXJobResponse{
		Namespace:   njreq.Namespace,
		Coordinator: njreq.Coordinator,
		Collectors:  delCollectors,
		Learners:    delLearners,
	}

	return rep, nil
}

func (s *NerveXServer) getNerveXJob(namespace, coordinatorName string) (*nervexv1alpha1.NerveXJob, error) {
	// get coordinator
	coorKey := nervexutil.NamespacedName(namespace, coordinatorName)
	coordinator, err := s.getPodByKey(coorKey)
	if err != nil {
		return nil, err
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
		errMsg := fmt.Sprintf("coordinator %s is not owned by any NerveXJob", coordinatorName)
		return nil, &NerveXError{Type: ErrorNotFound, Message: errMsg}
	}

	// get NerveXJob
	njKey := nervexutil.NamespacedName(namespace, ownRefer.Name)
	nvxJob, err := s.getNerveXJobByKey(njKey)
	if err != nil {
		return nil, err
	}

	return nvxJob, nil
}

func (s *NerveXServer) getNerveXJobByKey(key string) (*nervexv1alpha1.NerveXJob, error) {
	obj, exists, err := s.dyi.NJInformer.Informer().GetIndexer().GetByKey(key)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get NerveXJob: %s", err)
		return nil, fmt.Errorf(errMsg)
	}

	if !exists {
		errMsg := fmt.Sprintf("NerveXJob: %s not exists in cache", key)
		return nil, &NerveXError{Type: ErrorNotFound, Message: errMsg}
	}
	nvxUn := obj.(*unstructured.Unstructured)
	var nvxJob nervexv1alpha1.NerveXJob
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(nvxUn.UnstructuredContent(), &nvxJob)
	if err != nil {
		errMsg := fmt.Sprintf("failed to convert unstructured: %s", nvxUn.UnstructuredContent())
		return nil, fmt.Errorf(errMsg)
	}

	return &nvxJob, nil
}

func (s *NerveXServer) getPodByKey(key string) (*corev1.Pod, error) {
	obj, exists, err := s.dyi.PodInformer.Informer().GetIndexer().GetByKey(key)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get pod: %s", err)
		return nil, fmt.Errorf(errMsg)
	}
	if !exists {
		errMsg := fmt.Sprintf("pod: %s not exists in cache", key)
		return nil, &NerveXError{Type: ErrorNotFound, Message: errMsg}
	}

	podUn := obj.(*unstructured.Unstructured)
	var pod corev1.Pod
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(podUn.UnstructuredContent(), &pod)
	if err != nil {
		errMsg := fmt.Sprintf("failed to convert unstructured: %s", podUn.UnstructuredContent())
		return nil, fmt.Errorf(errMsg)
	}

	return &pod, nil
}

func writeResponse(w http.ResponseWriter, rep Response, statusCode int) error {
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

func (s NerveXServer) writeResponseToPod(rep NerveXJobResponse, njreq NerveXJobRequest, path string) error {
	log := s.Log.WithName("NerveXServer")

	// get coordinator
	coorKey := nervexutil.NamespacedName(njreq.Namespace, njreq.Coordinator)
	coor, err := s.getPodByKey(coorKey)
	if err != nil {
		return err
	}

	port, found := nervexutil.GetPortFromPod(coor, nervexutil.DefaultCoordinatorContainerName, nervexutil.DefaultCoordinatorPortName)
	if !found {
		port = nervexutil.DefaultCoordinatorPort
	}
	coorURL := nervexutil.ConcatURL(coor.Name, coor.Namespace, port)
	reqJSON, err := json.Marshal(rep)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("http://%s%s", coorURL, path)
	log.Info("send response to", "url", url)
	_, err = http.Post(url, "application/json", bytes.NewReader(reqJSON))
	if err != nil {
		return fmt.Errorf("failed to send request to coordinator %s: %v", url, err)
	}

	return nil
}

func (s *NerveXServer) listReplicaPodsWithSelector(ns string, labelSelector labels.Selector) (
	collectors []*corev1.Pod, learners []*corev1.Pod, coordinator *corev1.Pod, aggregator *corev1.Pod, err error) {
	// list pods that belong to the NerveXJob
	pods, err := s.listPodsWithSelector(ns, labelSelector)
	if err != nil {
		return
	}

	// classify pods
	collectors, learners, coordinator, aggregator, err = nervexutil.ClassifyPods(pods)
	if err != nil {
		return
	}
	return
}

func (s *NerveXServer) createCollectorsAndLearnersFromNerveXJob(
	njreq *NerveXJobRequest,
	job *nervexv1alpha1.NerveXJob) ([]string, []string, error) {

	// build owner reference
	ownRefer := metav1.OwnerReference{
		APIVersion: job.APIVersion,
		Kind:       job.Kind,
		Name:       job.Name,
		UID:        job.GetUID(),
		Controller: func(c bool) *bool { return &c }(true),
	}

	// create collectors
	collectorTemplate := job.Spec.Collector.Template
	collectors, err := s.createPodsAndServices(&collectorTemplate, ownRefer, njreq,
		nervexutil.CollectorName, nervexutil.DefaultCollectorContainerName, nervexutil.DefaultCollectorPortName, nervexutil.DefaultCollectorPort)

	if err != nil {
		return collectors, nil, err
	}

	// create learners
	learnerTemplate := job.Spec.Learner.Template
	learners, err := s.createPodsAndServices(&learnerTemplate, ownRefer, njreq,
		nervexutil.LearnerName, nervexutil.DefaultLearnerContainerName, nervexutil.DefaultLearnerPortName, nervexutil.DefaultLearnerPort)

	if err != nil {
		return collectors, learners, err
	}

	return collectors, learners, nil
}

func (s *NerveXServer) createPodsAndServices(
	template *corev1.PodTemplateSpec,
	ownRefer metav1.OwnerReference,
	njreq *NerveXJobRequest,
	replicaType, containerName, portName string, defaultPort int32) ([]string, error) {

	ns := njreq.Namespace
	resources := ResourceQuantity{}
	switch replicaType {
	case nervexutil.CollectorName:
		resources = njreq.Collectors
	case nervexutil.LearnerName:
		resources = njreq.Learners
	}

	results := []string{}
	// create pods and services
	for i := 0; i < resources.Replicas; i++ {
		// build pod
		pod, port, err := nervexutil.BuildPodFromTemplate(template.DeepCopy(), ownRefer, ns, replicaType, containerName, portName, defaultPort)
		if err != nil {
			return results, err
		}
		// set pod resources
		SetPodResources(pod, resources, containerName)

		// create pod
		_, err = s.KubeClient.CoreV1().Pods(ns).Create(context.Background(), pod, metav1.CreateOptions{})
		if err != nil {
			if k8serrors.IsAlreadyExists(err) {
				return results, &NerveXError{Type: ErrorAlreadyExists, Message: err.Error()}
			}
			return results, err
		}

		// build service
		svc := nervexutil.BuildService(pod.GetLabels(), port, portName)
		svc.SetOwnerReferences([]metav1.OwnerReference{ownRefer})
		svc.Name = pod.Name

		// create service
		_, err = s.KubeClient.CoreV1().Services(ns).Create(context.Background(), svc, metav1.CreateOptions{})
		if err != nil {
			if k8serrors.IsAlreadyExists(err) {
				return results, &NerveXError{Type: ErrorAlreadyExists, Message: err.Error()}
			}
			return results, err
		}

		result := nervexutil.ConcatURL(svc.Name, ns, port)
		results = append(results, result)
	}

	return results, nil
}

func (s *NerveXServer) listPodsWithSelector(namespace string, labelSelector labels.Selector) ([]*corev1.Pod, error) {
	ret, err := s.dyi.PodInformer.Lister().ByNamespace(namespace).List(labelSelector)
	if err != nil {
		return nil, err
	}

	pods := []*corev1.Pod{}
	for _, obj := range ret {
		podUn := obj.(*unstructured.Unstructured)
		var pod corev1.Pod
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(podUn.UnstructuredContent(), &pod); err != nil {
			return nil, err
		}
		pods = append(pods, &pod)
	}

	return pods, nil
}

func (s *NerveXServer) DeletePodsAndServices(pods []*corev1.Pod, njreq *NerveXJobRequest, replicaType string) ([]string, error) {
	results := []string{}
	resources := ResourceQuantity{}
	var containerName, portName string
	var defaultPort int32
	ns := njreq.Namespace

	switch replicaType {
	case nervexutil.CollectorName:
		resources = njreq.Collectors
		containerName = nervexutil.DefaultCollectorContainerName
		portName = nervexutil.DefaultCollectorPortName
		defaultPort = nervexutil.DefaultCollectorPort
	case nervexutil.LearnerName:
		resources = njreq.Learners
		containerName = nervexutil.DefaultLearnerContainerName
		portName = nervexutil.DefaultLearnerPortName
		defaultPort = nervexutil.DefaultLearnerPort
	default:
		return nil, &NerveXError{Type: ErrorBadRequest, Message: fmt.Sprintf("replica type %s is not supported", replicaType)}
	}

	for _, pod := range pods {
		// break if enough
		if len(results) >= resources.Replicas {
			break
		}

		// delete pods
		err := s.KubeClient.CoreV1().Pods(njreq.Namespace).Delete(context.Background(), pod.Name, metav1.DeleteOptions{})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return results, &NerveXError{Type: ErrorNotFound, Message: err.Error()}
			}
			return results, err
		}

		// delete services
		err = s.KubeClient.CoreV1().Services(njreq.Namespace).Delete(context.Background(), pod.Name, metav1.DeleteOptions{})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return results, &NerveXError{Type: ErrorNotFound, Message: err.Error()}
			}
			return results, err
		}

		result := nervexutil.GetPodAccessURL(pod, ns, containerName, portName, defaultPort)
		results = append(results, result)
	}

	return results, nil
}
