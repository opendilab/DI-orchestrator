package testutils

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dicommon "opendilab.org/di-orchestrator/pkg/common"
)

func NewPod(name, jobName string, ownRefer metav1.OwnerReference) *corev1.Pod {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: DIJobNamespace,
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

func UpdatePodPhase(ctx context.Context, k8sClient client.Client, podKey types.NamespacedName, phase corev1.PodPhase) error {
	var pod corev1.Pod
	err := k8sClient.Get(ctx, podKey, &pod)
	if err != nil {
		return err
	}

	containerName := pod.Spec.Containers[0].Name
	pod.Status.Phase = phase
	if phase == corev1.PodRunning {
		state := corev1.ContainerStateRunning{}
		cstatus := corev1.ContainerStatus{Name: containerName, State: corev1.ContainerState{
			Running: &state,
		}}
		pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, cstatus)
	}

	err = k8sClient.Status().Update(ctx, &pod, &client.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func GetPodLogs(clientSet *kubernetes.Clientset,
	namespace string, podName string, containerName string, follow bool, message string, timeout time.Duration) error {
	count := int64(100)
	podLogOptions := corev1.PodLogOptions{
		Container: containerName,
		Follow:    follow,
		TailLines: &count,
	}

	podLogRequest := clientSet.CoreV1().
		Pods(namespace).
		GetLogs(podName, &podLogOptions)
	stream, err := podLogRequest.Stream(context.TODO())
	if err != nil {
		return err
	}
	defer stream.Close()

	err = wait.Poll(50*time.Millisecond, timeout, func() (bool, error) {
		buf := make([]byte, 2000)
		numBytes, err := stream.Read(buf)
		if numBytes == 0 {
			return false, nil
		}
		if err == io.EOF {
			return false, fmt.Errorf("can't find related logs '%s' from pod %s", message, podName)
		}
		if err != nil {
			return false, err
		}
		logs := string(buf[:numBytes])
		if strings.Contains(logs, message) {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return err
	}
	return nil
}
