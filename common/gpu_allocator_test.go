package common

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
)

func TestAllocators(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Allocator Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = Describe("Test SimpleGPUAllocator", func() {
	It("ValueOccursMostFrequentInList function", func() {
		testCases := map[string]struct {
			list          []int
			expectedValue int
			expectedCount int
		}{
			"Only one max value": {
				[]int{1, 2, 3, 4, 5, 2, 2, 4, 6, 2, 3, 3, 1},
				2, 4,
			},
			"Multi max value": {
				[]int{1, 2, 3, 4, 5, 2, 2, 4, 6, 2, 3, 3, 1, 3},
				3, 4,
			},
		}
		for _, test := range testCases {
			maxValue, maxCount := ValueOccursMostFrequentInList(test.list)
			Expect(maxValue).To(Equal(test.expectedValue))
			Expect(maxCount).To(Equal(test.expectedCount))
		}
	})

	It("Allocate function", func() {
		testCases := map[string]struct {
			nodeGPUs map[int]int
			gpus     int
			result   []int
		}{
			"Only one max value with 12 gpus request": {
				map[int]int{
					8:  4,
					10: 3,
					6:  3,
				},
				12, []int{8, 4},
			},
			"Only one max value with 16 gpus request": {
				map[int]int{
					8:  4,
					10: 3,
					6:  3,
				},
				16, []int{8, 8},
			},
			"Multi max value with 16 gpus request": {
				map[int]int{
					8:  4,
					10: 4,
					6:  3,
				},
				16, []int{10, 6},
			},
			"Multi max value with 8 gpus request": {
				map[int]int{
					8:  4,
					10: 4,
					6:  3,
				},
				8, []int{8},
			},
		}
		for _, test := range testCases {
			var nodes []*corev1.Node
			for nodeSpec, nodeGPUs := range test.nodeGPUs {
				for i := 0; i < nodeGPUs; i++ {
					nodes = append(nodes, newNode(nodeSpec))
				}
			}

			alloc := NewSimpleGPUAllocator(nodes)
			result := alloc.Allocate(test.gpus)
			Expect(result).To(Equal(test.result))
		}
	})
})

func newNode(gpus int) *corev1.Node {
	return &corev1.Node{
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				"nvidia.com/gpu": *resource.NewQuantity(int64(gpus), resource.DecimalExponent),
			},
		},
	}
}
