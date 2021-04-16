package http

import corev1 "k8s.io/api/core/v1"

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
		if !resources.Cpu.IsZero() {
			template.Spec.Containers[i].Resources.Limits[corev1.ResourceCPU] = resources.Cpu
			template.Spec.Containers[i].Resources.Requests[corev1.ResourceCPU] = resources.Cpu
		}
		if !resources.Memory.IsZero() {
			template.Spec.Containers[i].Resources.Limits[corev1.ResourceMemory] = resources.Memory
			template.Spec.Containers[i].Resources.Requests[corev1.ResourceMemory] = resources.Memory
		}
		if !resources.Gpu.IsZero() {
			template.Spec.Containers[i].Resources.Limits[corev1.ResourceName("nvidia.com/gpu")] = resources.Gpu
			template.Spec.Containers[i].Resources.Requests[corev1.ResourceName("nvidia.com/gpu")] = resources.Gpu
		}
	}
}
