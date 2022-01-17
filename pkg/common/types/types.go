package types

import (
	"k8s.io/apimachinery/pkg/api/resource"
)

type DIJobRequestParams struct {
	Namespace   []string `json:"namespace"`
	Coordinator []string `json:"coordinator"`
	Name        []string `json:"name"`
	Aggregator  []string `json:"aggregator"`
}

const (
	RequestParamTypeNamespace   string = "namespace"
	RequestParamTypeCoordinator string = "coordinator"
	RequestParamTypeName        string = "name"
	RequestParamTypeAggregator  string = "aggregator"
)

type DIJobRequest struct {
	Namespace   string           `json:"namespace"`
	Coordinator string           `json:"coordinator"`
	Collectors  ResourceQuantity `json:"collectors"`
	Learners    ResourceQuantity `json:"learners"`
}

type ResourceQuantity struct {
	Replicas int               `json:"replicas"`
	CPU      resource.Quantity `json:"cpus"`
	GPU      resource.Quantity `json:"gpus"`
	Memory   resource.Quantity `json:"memory"`
}

func (in *ResourceQuantity) DeepCopy() *ResourceQuantity {
	out := &ResourceQuantity{}
	out.Replicas = in.Replicas
	out.CPU = in.CPU.DeepCopy()
	out.GPU = in.GPU.DeepCopy()
	out.Memory = in.Memory.DeepCopy()
	return out
}

func NewResourceQuantity(replicas int, cpu, gpu, memory string) *ResourceQuantity {
	return &ResourceQuantity{
		Replicas: replicas,
		CPU:      resource.MustParse(cpu),
		GPU:      resource.MustParse(gpu),
		Memory:   resource.MustParse(memory),
	}
}

type Response struct {
	Success bool        `json:"success"`
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

const (
	CodeSuccess = iota
	CodeFailed
)

type DIJobResponse struct {
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
