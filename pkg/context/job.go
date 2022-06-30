package context

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
)

func (c *Context) CleanUpJob(ctx context.Context, job *div2alpha1.DIJob) error {
	err := c.Delete(ctx, job, &client.DeleteOptions{})
	if err != nil {
		return err
	}
	time.Sleep(250 * time.Millisecond)

	pods, err := c.ListJobPods(ctx, job)
	if err != nil {
		return err
	}
	for _, pod := range pods {
		err = c.Delete(ctx, pod, &client.DeleteOptions{GracePeriodSeconds: func(a int64) *int64 { return &a }(0)})
		if err != nil {
			return err
		}
	}

	svcs, err := c.ListJobServices(ctx, job)
	if err != nil {
		return err
	}
	for _, svc := range svcs {
		err = c.Delete(ctx, svc, &client.DeleteOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Context) WaitForAllReplicas(ctx context.Context, job *div2alpha1.DIJob, phase corev1.PodPhase) error {
	if err := wait.Poll(100*time.Millisecond, 5*time.Minute, func() (bool, error) {
		pods, err := c.ListJobPods(ctx, job)
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
