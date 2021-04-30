package http

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	serverdynamic "go-sensephoenix.sensetime.com/nervex-operator/server/dynamic"
)

type NerveXServer struct {
	KubeClient    *kubernetes.Clientset
	DynamicClient dynamic.Interface
	Log           logr.Logger
	dyi           serverdynamic.Informers
}

type NerveXJobRequestParams struct {
	Namespace   []string `json:"namespace"`
	Coordinator []string `json:"coordinator"`
	Name        []string `json:"name"`
}

const (
	RequestParamTypeNamespace   string = "namespace"
	RequestParamTypeCoordinator string = "coordinator"
	RequestParamTypeName        string = "name"
)

type NerveXJobRequest struct {
	Namespace   string           `json:"namespace"`
	Coordinator string           `json:"coordinator"`
	Collectors  ResourceQuantity `json:"collectors"`
	Learners    ResourceQuantity `json:"learners"`
}

type ResourceQuantity struct {
	Replicas int               `json:"replicas"`
	CPU      resource.Quantity `json:"cpus"`
	Gpu      resource.Quantity `json:"gpus"`
	Memory   resource.Quantity `json:"memory"`
}

type Response struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}
type NerveXJobResponse struct {
	Namespace   string   `json:"namespace"`
	Coordinator string   `json:"coordinator"`
	Collectors  []string `json:"collectors"`
	Learners    []string `json:"learners"`
}

type ReplicaResponse struct {
	Namespace   string `json:"namespace"`
	Coordinator string `json:"coordinator"`
	ReplicaType string `json:"replicaType"`
	Name        string `json:"name"`
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
