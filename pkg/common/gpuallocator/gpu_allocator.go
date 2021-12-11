package gpu_allocator

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

type GPUAllocator struct {
	Nodes []*corev1.Node

	policy Policy
}

func NewSimpleGPUAllocator(nodes []*corev1.Node) *GPUAllocator {
	return &GPUAllocator{Nodes: nodes, policy: NewSimplePolicy()}
}

func (g *GPUAllocator) Allocate(gpus int) []int {
	return g.policy.Allocate(g.Nodes, gpus)
}

func (g *GPUAllocator) NumGPUsOfMajorityNodeType() int {
	return GetGPUsMajority(g.Nodes)
}

const (
	SimpleGPUAllocPolicy = "simple"
)

type Policy interface {
	Allocate(nodes []*corev1.Node, gpus int) []int
}

type SimplePolicy struct{}

func NewSimplePolicy() *SimplePolicy {
	return &SimplePolicy{}
}

func (s *SimplePolicy) Allocate(nodes []*corev1.Node, gpus int) []int {
	// gpusMajority is the node gpus with most frequent occurrence.
	// maxGPUCount is the number of nodes with gpus equal to gpusMajority
	gpusMajority := GetGPUsMajority(nodes)
	if gpusMajority <= 0 {
		return nil
	}
	perNodeGPUs := Max(gpusMajority, 1)

	if gpus < perNodeGPUs {
		return []int{gpus}
	}

	var result []int
	nResults := gpus / perNodeGPUs
	for i := 0; i < nResults; i++ {
		result = append(result, perNodeGPUs)
	}
	remainGPUs := gpus - nResults*perNodeGPUs
	if remainGPUs > 0 {
		result = append(result, remainGPUs)
	}
	return result
}

func Max(x, y int) int {
	if x < y {
		return y
	}
	return x
}

func MaxInArray(v []int) (int, error) {
	if len(v) == 0 {
		return 0, fmt.Errorf("empty list")
	}
	max := v[0]
	for _, i := range v {
		if i > max {
			max = i
		}
	}
	return max, nil
}

func GetGPUsMajority(nodes []*corev1.Node) int {
	var nodeGPUCounts []int
	for _, node := range nodes {
		allocGPUs := node.Status.Allocatable[corev1.ResourceName("nvidia.com/gpu")]
		nodeGPUCounts = append(nodeGPUCounts, int(allocGPUs.Value()))
	}

	// gpusMajority is the number of gpus of majority nodes.
	// majorityNodes is the number of nodes with gpus equal to gpusMajority
	gpusMajority, _ := ValueOccursMostFrequentInList(nodeGPUCounts)

	if gpusMajority == 0 {
		max, _ := MaxInArray(nodeGPUCounts)
		return max
	}
	return gpusMajority
}

// ValueOccursMostFrequentInList returns value that occurs most frequently in list,
// and the count of occurrences.
func ValueOccursMostFrequentInList(list []int) (int, int) {
	if len(list) == 0 {
		return 0, 0
	}

	// map the occurrence frequency of each value
	maxCount := 0
	maxCountValue := 0
	valuesMap := make(map[int]int)

	for _, v := range list {
		if valuesMap[v] != 0 {
			valuesMap[v]++
		} else {
			valuesMap[v] = 1
		}

		if maxCount < valuesMap[v] {
			maxCount = valuesMap[v]
			maxCountValue = v
		} else if maxCount == valuesMap[v] && maxCountValue < v {
			maxCountValue = v
		}
	}

	return maxCountValue, maxCount
}
