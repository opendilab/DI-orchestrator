package testutils

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func UpdatePodPhase(ctx context.Context, k8sClient client.Client, podKey types.NamespacedName, phase corev1.PodPhase) error {
	var pod corev1.Pod
	err := k8sClient.Get(ctx, podKey, &pod)
	if err != nil {
		return err
	}
	pod.Status.Phase = phase
	err = k8sClient.Status().Update(ctx, &pod, &client.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}
