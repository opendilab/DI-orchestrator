package testutils

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
)

func NewDIJob() *div2alpha1.DIJob {
	return &div2alpha1.DIJob{
		TypeMeta: metav1.TypeMeta{
			Kind:       div2alpha1.KindDIJob,
			APIVersion: div2alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      DIJobName,
			Namespace: DIJobNamespace,
		},
		Spec: div2alpha1.DIJobSpec{
			MinReplicas: 1,
			MaxReplicas: 4,
			Preemptible: false,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    dicommon.DefaultContainerName,
							Image:   DIJobImage,
							Command: []string{"/bin/sh", "-c", "sleep", DefaultSleepDuration},
						},
					},
				},
			},
		},
	}
}

func NewNamespace(namespace string) *corev1.Namespace {
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
}
