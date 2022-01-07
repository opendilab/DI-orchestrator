package testutils

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
)

func NewDIJob() *div1alpha2.DIJob {
	return &div1alpha2.DIJob{
		TypeMeta: metav1.TypeMeta{
			Kind:       div1alpha2.KindDIJob,
			APIVersion: div1alpha2.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      DIJobName,
			Namespace: DIJobNamespace,
		},
		Spec: div1alpha2.DIJobSpec{
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
