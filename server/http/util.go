package http

import (
	corev1 "k8s.io/api/core/v1"
)

func SetPodResources(template *corev1.Pod, resources ResourceQuantity, containerName string) {
	for i := range template.Spec.Containers {
		if template.Spec.Containers[i].Name != containerName {
			continue
		}
		if template.Spec.Containers[i].Resources.Limits == nil {
			template.Spec.Containers[i].Resources.Limits = make(corev1.ResourceList)
		}
		if template.Spec.Containers[i].Resources.Requests == nil {
			template.Spec.Containers[i].Resources.Requests = make(corev1.ResourceList)
		}

		// cpu and memory must not be zero
		if !resources.CPU.IsZero() {
			template.Spec.Containers[i].Resources.Limits[corev1.ResourceCPU] = resources.CPU
			template.Spec.Containers[i].Resources.Requests[corev1.ResourceCPU] = resources.CPU
		}
		if !resources.Memory.IsZero() {
			template.Spec.Containers[i].Resources.Limits[corev1.ResourceMemory] = resources.Memory
			template.Spec.Containers[i].Resources.Requests[corev1.ResourceMemory] = resources.Memory
		}
		if !resources.GPU.IsZero() {
			template.Spec.Containers[i].Resources.Limits[corev1.ResourceName("nvidia.com/gpu")] = resources.GPU
			template.Spec.Containers[i].Resources.Requests[corev1.ResourceName("nvidia.com/gpu")] = resources.GPU
		}
	}
}
