package testutils

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
	nervexutil "go-sensephoenix.sensetime.com/nervex-operator/utils"
)

func NewNerveXJob() *nervexv1alpha1.NerveXJob {
	return &nervexv1alpha1.NerveXJob{
		TypeMeta: metav1.TypeMeta{
			Kind:       nervexv1alpha1.KindNerveXJob,
			APIVersion: nervexv1alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      NerveXJobName,
			Namespace: NerveXJobNamespace,
		},
		Spec: nervexv1alpha1.NerveXJobSpec{
			Coordinator: nervexv1alpha1.CoordinatorSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:    nervexutil.DefaultCoordinatorContainerName,
								Image:   NerveXJobImage,
								Command: []string{"/bin/sh", "-c", "sleep", DefaultSleepDuration},
							},
						},
					},
				},
			},
			Collector: nervexv1alpha1.CollectorSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:    nervexutil.DefaultCollectorContainerName,
								Image:   NerveXJobImage,
								Command: []string{"/bin/sh", "-c", "sleep", DefaultSleepDuration},
							},
						},
					},
				},
			},
			Learner: nervexv1alpha1.LearnerSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:    nervexutil.DefaultLearnerContainerName,
								Image:   NerveXJobImage,
								Command: []string{"/bin/sh", "-c", "sleep", DefaultSleepDuration},
							},
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

func NewAggregatorConfig() *nervexv1alpha1.AggregatorConfig {
	return &nervexv1alpha1.AggregatorConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: nervexv1alpha1.GroupVersion.String(),
			Kind:       nervexv1alpha1.KindAGConfig,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      DefaultAGConfigName,
			Namespace: DefaultAGConfigNamespace,
		},
		Spec: nervexv1alpha1.AggregatorConfigSpec{
			Aggregator: nervexv1alpha1.AggregatorSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:    nervexutil.DefaultAggregatorContainerName,
								Image:   NerveXJobImage,
								Command: []string{"/bin/sh", "-c", "sleep", DefaultSleepDuration},
							},
						},
					},
				},
			},
		},
	}
}
