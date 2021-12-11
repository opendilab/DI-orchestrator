package util

// import (
// 	corev1 "k8s.io/api/core/v1"
// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// 	"k8s.io/apimachinery/pkg/labels"

// 	dicommon "opendilab.org/di-orchestrator/pkg/common"
// )

// func filterReplicaPods(pods []*corev1.Pod, replicaType string) ([]*corev1.Pod, error) {
// 	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
// 		MatchLabels: map[string]string{dicommon.ReplicaTypeLabel: replicaType},
// 	})
// 	if err != nil {
// 		return nil, err
// 	}

// 	result := []*corev1.Pod{}
// 	for _, pod := range pods {
// 		if !selector.Matches(labels.Set(pod.Labels)) {
// 			continue
// 		}
// 		result = append(result, pod)
// 	}
// 	return result, nil
// }

// func filterReplicaServices(services []*corev1.Service, replicaType string) ([]*corev1.Service, error) {
// 	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
// 		MatchLabels: map[string]string{dicommon.ReplicaTypeLabel: replicaType},
// 	})
// 	if err != nil {
// 		return nil, err
// 	}

// 	result := []*corev1.Service{}
// 	for _, service := range services {
// 		if !selector.Matches(labels.Set(service.Labels)) {
// 			continue
// 		}
// 		result = append(result, service)
// 	}
// 	return result, nil
// }
