package testutils

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nervexutil "go-sensephoenix.sensetime.com/nervex-operator/utils"
)

func NewPod(name, jobName string, ownRefer metav1.OwnerReference) *corev1.Pod {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: NerveXJobNamespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    nervexutil.DefaultContainerName,
					Image:   NerveXJobImage,
					Command: []string{"/bin/sh", "-c", "sleep", DefaultSleepDuration},
				},
			},
		},
	}
	pod.SetOwnerReferences([]metav1.OwnerReference{ownRefer})
	return pod
}

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
