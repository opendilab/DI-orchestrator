package http

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type NerveXServer struct {
	KubeClient    *kubernetes.Clientset
	DynamicClient dynamic.Interface
	Log           logr.Logger
	alDyi         cache.SharedIndexInformer
	njDyi         cache.SharedIndexInformer
	alconfig      string
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
	alDyi cache.SharedIndexInformer,
	njDyi cache.SharedIndexInformer,
	alconfig string) *NerveXServer {

	return &NerveXServer{
		KubeClient:    kubeClient,
		DynamicClient: dynamicClient,
		Log:           log,
		alDyi:         alDyi,
		njDyi:         njDyi,
		alconfig:      alconfig,
	}
}
