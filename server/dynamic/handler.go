package dynamic

import (
	"fmt"
	"log"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func GetPodFromObject(obj interface{}) (*corev1.Pod, error) {
	podUn := obj.(*unstructured.Unstructured)
	var pod corev1.Pod
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(podUn.UnstructuredContent(), &pod); err != nil {
		log.Printf("failed to convert pod %v", err)
		return nil, err
	}
	owner := metav1.GetControllerOf(&pod)
	if owner == nil || owner.Kind != nervexv1alpha1.KindNerveXJob {
		return nil, fmt.Errorf("pod %s not belong to NerveXJob", pod.Name)
	}

	return &pod, nil
}

// func GetRestartCountFromPod(pod *corev1.Pod) int {

// }

func addPodHandler(obj interface{}) {
	// on add object
	pod, err := GetPodFromObject(obj)
	if err != nil {
		return
	}
	log.Printf("new pod: %s/%s", pod.GetNamespace(), pod.GetName())
}

func updatePodHandler(old, new interface{}) {
	// // on update object
	// newpod, err := GetPodFromObject(new)
	// if err != nil {
	// 	log.Printf("failed to get pod: %v", err)
	// 	return
	// }

	// restartcount := newpod.Status.ContainerStatuses
}
