package types

import (
	"k8s.io/apimachinery/pkg/api/resource"
)

type DIJobRequest struct {
	Replicas int `json:"replicas"`
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
	Success bool     `json:"success"`
	Code    int      `json:"code"`
	Message string   `json:"message"`
	Data    []string `json:"data"`
}

const (
	CodeSuccess = iota
	CodeFailed
)
