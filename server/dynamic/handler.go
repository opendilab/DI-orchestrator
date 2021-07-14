package dynamic

import (
	"fmt"
	"log"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	div1alpha1 "opendilab.org/di-orchestrator/api/v1alpha1"
)

func GetPodFromObject(obj interface{}) (*corev1.Pod, error) {
	podUn := obj.(*unstructured.Unstructured)
	var pod corev1.Pod
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(podUn.UnstructuredContent(), &pod); err != nil {
		log.Printf("failed to convert pod %v", err)
		return nil, err
	}
	owners := pod.GetOwnerReferences()
	for _, owner := range owners {
		if owner.Kind == div1alpha1.KindDIJob {
			return &pod, nil
		}
	}
	return nil, fmt.Errorf("pod %s not belong to DIJob", pod.Name)
}

func GetServiceFromObject(obj interface{}) (*corev1.Service, error) {
	svcUn := obj.(*unstructured.Unstructured)
	var service corev1.Service
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(svcUn.UnstructuredContent(), &service); err != nil {
		log.Printf("failed to convert service %v", err)
		return nil, err
	}
	owners := service.GetOwnerReferences()
	for _, owner := range owners {
		if owner.Kind == div1alpha1.KindDIJob {
			return &service, nil
		}
	}
	return nil, fmt.Errorf("service %s not belong to DIJob", service.Name)
}

func isNotBelongToDIJobError(err error) bool {
	if strings.Contains(err.Error(), "not belong to DIJob") {
		return true
	}
	return false
}
