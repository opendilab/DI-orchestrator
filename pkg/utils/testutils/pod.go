package testutils

import (
	"bytes"
	"context"
	"io"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dicommon "opendilab.org/di-orchestrator/pkg/common"
	dicontext "opendilab.org/di-orchestrator/pkg/context"
)

func NewPod(name, namespace string, ownRefer metav1.OwnerReference) *corev1.Pod {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    dicommon.DefaultContainerName,
					Image:   DIJobImage,
					Command: []string{"/bin/sh", "-c", "sleep", DefaultSleepDuration},
				},
			},
		},
	}
	pod.SetOwnerReferences([]metav1.OwnerReference{ownRefer})
	return pod
}

func UpdatePodPhase(ctx dicontext.Context, podKey types.NamespacedName, phase corev1.PodPhase) error {
	var pod corev1.Pod
	err := ctx.Get(context.TODO(), podKey, &pod)
	if err != nil {
		return err
	}

	containerName := pod.Spec.Containers[0].Name
	pod.Status.Phase = phase
	if phase == corev1.PodRunning {
		state := corev1.ContainerStateRunning{}
		cstatus := corev1.ContainerStatus{Name: containerName, State: corev1.ContainerState{
			Running: &state,
		}, Ready: true}
		pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, cstatus)
	}

	err = ctx.Status().Update(context.TODO(), &pod, &client.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func GetPodLogs(clientSet *kubernetes.Clientset,
	namespace string, podName string, containerName string, follow bool) (string, error) {
	podLogOptions := corev1.PodLogOptions{
		Container: containerName,
		Follow:    follow,
	}

	podLogRequest := clientSet.CoreV1().
		Pods(namespace).
		GetLogs(podName, &podLogOptions)
	stream, err := podLogRequest.Stream(context.TODO())
	if err != nil {
		return "", err
	}
	defer stream.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, stream)
	if err != nil {
		return "", err
	}
	str := buf.String()
	return str, nil
}
