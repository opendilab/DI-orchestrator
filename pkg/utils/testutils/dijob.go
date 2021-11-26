package testutils

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div1alpha1 "opendilab.org/di-orchestrator/pkg/api/v1alpha1"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

func NewDIJob() *div1alpha1.DIJob {
	return &div1alpha1.DIJob{
		TypeMeta: metav1.TypeMeta{
			Kind:       div1alpha1.KindDIJob,
			APIVersion: div1alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      DIJobName,
			Namespace: DIJobNamespace,
		},
		Spec: div1alpha1.DIJobSpec{
			Coordinator: div1alpha1.CoordinatorSpec{
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
			Collector: div1alpha1.CollectorSpec{
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
			Learner: div1alpha1.LearnerSpec{
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

func NewAggregatorConfig() *div1alpha1.AggregatorConfig {
	return &div1alpha1.AggregatorConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: div1alpha1.GroupVersion.String(),
			Kind:       div1alpha1.KindAGConfig,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      DefaultAGConfigName,
			Namespace: DefaultAGConfigNamespace,
		},
		Spec: div1alpha1.AggregatorConfigSpec{
			Aggregator: div1alpha1.AggregatorSpec{
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
		},
	}
}

func CleanUpJob(ctx context.Context, k8sClient client.Client, job *div1alpha1.DIJob) error {
	err := k8sClient.Delete(ctx, job, &client.DeleteOptions{})
	if err != nil {
		return err
	}

	time.Sleep(250 * time.Millisecond)

	pods, err := diutil.ListPods(ctx, k8sClient, job)
	if err != nil {
		return err
	}

	for _, pod := range pods {
		err = k8sClient.Delete(ctx, pod, &client.DeleteOptions{GracePeriodSeconds: func(a int64) *int64 { return &a }(0)})
		if err != nil {
			return err
		}
	}

	svcs, err := diutil.ListServices(ctx, k8sClient, job)
	if err != nil {
		return err
	}

	for _, svc := range svcs {
		err = k8sClient.Delete(ctx, svc, &client.DeleteOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func WaitForAllReplicas(ctx context.Context, k8sClient client.Client, job *div1alpha1.DIJob, phase corev1.PodPhase) error {
	if err := wait.Poll(100*time.Millisecond, 5*time.Minute, func() (bool, error) {
		pods, err := diutil.ListPods(ctx, k8sClient, job)
		if err != nil {
			return false, err
		}

		// if there are only coordinator, keep waiting
		if len(pods) <= 1 {
			return false, nil
		}

		for _, pod := range pods {
			if pod.Status.Phase != phase {
				return false, nil
			}
		}

		return true, nil
	}); err != nil {
		return err
	}

	return nil
}
