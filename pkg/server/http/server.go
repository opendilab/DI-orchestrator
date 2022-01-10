package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	dicommon "opendilab.org/di-orchestrator/pkg/common"
	gpualloc "opendilab.org/di-orchestrator/pkg/common/gpuallocator"
	commontypes "opendilab.org/di-orchestrator/pkg/common/types"
	serverdynamic "opendilab.org/di-orchestrator/pkg/server/dynamic"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

var (
	apiVersion  = "v1alpha2"
	replicasAPI = "/replicas"
)

func withAPIVersion(api string) string {
	return fmt.Sprintf("/%s%s", apiVersion, api)
}

type DIServer struct {
	KubeClient   *kubernetes.Clientset
	DIClient     dynamic.NamespaceableResourceInterface
	Log          logr.Logger
	dyi          serverdynamic.Informers
	gpuAllocator gpualloc.GPUAllocator
}

func NewDIServer(
	kubeClient *kubernetes.Clientset,
	diClient dynamic.NamespaceableResourceInterface,
	log logr.Logger,
	dyi serverdynamic.Informers,
	gpuAllocPolicy string) *DIServer {

	var gpuAllocator gpualloc.GPUAllocator
	switch gpuAllocPolicy {
	case gpualloc.SimpleGPUAllocPolicy:
		gpuAllocator = *gpualloc.NewSimpleGPUAllocator([]*corev1.Node{})
	}
	return &DIServer{
		KubeClient:   kubeClient,
		DIClient:     diClient,
		Log:          log,
		dyi:          dyi,
		gpuAllocator: gpuAllocator,
	}
}

func (s *DIServer) Start(serverBindAddress string) error {
	log := s.Log.WithName("DIServer")
	http.HandleFunc(withAPIVersion(replicasAPI), s.Replicas)
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
	log := s.Log.WithName("Replicas")

	var reps []string
	var err error
	var msg string

	// handle request by request method
	switch r.Method {
	case "GET":
		msg = "successfully get replicas"
		reps, err = s.getReplicas(r)
	case "POST":
		msg = "successfully create replicas"
		err = s.addReplicas(r)
	case "DELETE":
		msg = "successfully delete replicas"
		err = s.deleteReplicas(r)
	default:
		err = &commontypes.DIError{Type: commontypes.ErrorNotImplemented, Message: fmt.Sprintf("%s not implemented", r.Method)}
		log.Error(err, "method not implemented")
	}

	rep, statusCode := s.buildResponse(reps, msg, err)

	// write response
	if err = writeResponse(w, rep, statusCode); err != nil {
		log.Error(err, "failed to write response")
	}
}

func (s *DIServer) getReplicas(r *http.Request) ([]string, error) {
	// get request params from request
	rp := commontypes.DIJobRequestParams{}
	params := r.URL.Query()
	for k, v := range params {
		switch strings.ToLower(k) {
		case commontypes.RequestParamTypeJobID:
			rp.JobID = v
		case commontypes.RequestParamTypeGeneration:
			rp.Generation = v
		default:
			errInfo := fmt.Sprintf("request param %s is not supported", k)
			return nil, &commontypes.DIError{Type: commontypes.ErrorBadRequest, Message: errInfo}
		}
	}

	var reps []string
	var err error
	if rp.JobID != nil && rp.Generation != nil {
		reps, err = s.getNamespacedReplicas(rp.JobID[0], rp.Generation[0])
		if err != nil {
			return nil, err
		}
	}

	return reps, nil
}

func (s *DIServer) getNamespacedReplicas(jobID string, generation string) ([]string, error) {
	log := s.Log.WithName("getNamespacedReplicas")

	job, err := s.getCachedDIJobByKey(jobID)
	if err != nil {
		log.Error(err, "failed to get owner reference")
		return nil, err
	}

	// list pods that belong to the DIJob
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: diutil.GenLabels(*job),
	})
	if err != nil {
		return nil, err
	}
	pods, err := s.listReplicaPodsWithSelector(jobID, labelSelector)
	if err != nil {
		log.Error(err, "failed to list collectors and learners")
		return nil, err
	}

	// get access urls
	var urls []string
	for _, pod := range pods {
		if pod.Status.PodIP == "" || pod.Annotations[dicommon.AnnotationGeneration] != generation {
			continue
		}
		replicas, _ := strconv.Atoi(pod.Annotations[dicommon.AnnotationReplicas])
		rank, _ := strconv.Atoi(pod.Annotations[dicommon.AnnotationRank])
		if urls == nil {
			urls = make([]string, replicas)
		}
		port, found := diutil.GetDefaultPortFromPod(pod)
		if !found {
			port = dicommon.DefaultPort
		}
		podIP := pod.Status.PodIP
		url := fmt.Sprintf("%s:%d", podIP, port)
		urls[rank] = url
	}

	log.Info("get replicas", "url", urls)
	return urls, nil
}

// add replicas api
func (s *DIServer) addReplicas(r *http.Request) error {
	log := s.Log.WithName("addReplicas")
	// get request body
	var req commontypes.DIJobRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode request body: %v", err)
		return &commontypes.DIError{Type: commontypes.ErrorBadRequest, Message: errMsg}
	}

	job, err := s.getCachedDIJobByKey(req.JobID)
	if err != nil {
		return err
	}

	// add replicas
	if job.Spec.Preemptible {
		oldStatus := job.Status.DeepCopy()
		job.Status.Replicas += int32(req.Replicas)
		if !apiequality.Semantic.DeepEqual(*oldStatus, job.Status) {
			if err := s.updateDIJobStatusInCluster(job); err != nil {
				log.Error(err, "failed to update DIJobStatus", "job", job.Name)
				return err
			}
		}
	}
	log.Info("successfully add replicas")
	return nil
}

// delete replicas api
func (s *DIServer) deleteReplicas(r *http.Request) error {
	log := s.Log.WithName("addReplicas")
	// get request body
	var req commontypes.DIJobRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode request body: %v", err)
		return &commontypes.DIError{Type: commontypes.ErrorBadRequest, Message: errMsg}
	}

	job, err := s.getCachedDIJobByKey(req.JobID)
	if err != nil {
		return err
	}

	// delete replicas
	if job.Spec.Preemptible {
		oldStatus := job.Status.DeepCopy()
		job.Status.Replicas -= int32(req.Replicas)
		if !apiequality.Semantic.DeepEqual(*oldStatus, job.Status) {
			if err := s.updateDIJobStatusInCluster(job); err != nil {
				log.Error(err, "failed to update DIJobStatus", "job", job.Name)
				return err
			}
		}
	}
	log.Info("successfully delete replicas")
	return nil
}

func (s *DIServer) buildResponse(reps []string, msg string, err error) (commontypes.Response, int) {
	log := s.Log.WithName("DIServer")

	var success bool = true
	var code int = commontypes.CodeSuccess
	var statusCode int = http.StatusOK
	if err != nil {
		success = false
		code = commontypes.CodeFailed
		msg = err.Error()

		// define status code
		if commontypes.IsNotFound(err) {
			statusCode = http.StatusNotFound
		} else if commontypes.IsAlreadyExists(err) {
			statusCode = http.StatusConflict
		} else if commontypes.IsBadRequest(err) {
			statusCode = http.StatusBadRequest
		} else if commontypes.IsNotImplemented(err) {
			statusCode = http.StatusNotImplemented
		} else {
			statusCode = http.StatusInternalServerError
		}

		log.Error(err, "failed to process request")
	}

	// build response
	rep := commontypes.Response{
		Success: success,
		Code:    code,
		Message: msg,
		Data:    reps,
	}
	return rep, statusCode
}

func writeResponse(w http.ResponseWriter, rep commontypes.Response, statusCode int) error {
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
