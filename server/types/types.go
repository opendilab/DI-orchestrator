package types

import (
	"k8s.io/apimachinery/pkg/api/resource"
)

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
