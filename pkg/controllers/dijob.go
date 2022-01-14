package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"

	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
	dicontext "opendilab.org/di-orchestrator/pkg/context"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

func (r *DIJobReconciler) reconcileReplicas(ctx context.Context, job *div1alpha2.DIJob,
	pods []*corev1.Pod, services []*corev1.Service) error {
	log := r.ctx.Log.WithName("reconcileReplicas").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	oldStatus := job.Status.DeepCopy()
	allocation := job.Status.Allocation
	replicas := job.Status.Replicas
	if r.ctx.CheckJobCompletion(job, pods) {
		if !apiequality.Semantic.DeepEqual(*oldStatus, job.Status) {
			if err := r.ctx.UpdateDIJobStatusInCluster(job); err != nil {
				log.Error(err, "failed to update job status")
				return err
			}
		}
		return nil
	}

	if err := r.reconcileWithJobStatus(job, pods); err != nil {
		return err
	}

	if err := r.reconcileServices(job, services); err != nil {
		return err
	}

	if job.Spec.Preemptible {
		if allocation != nil {
			job.Status.Replicas = int32(len(allocation))
			job.Status.ReadyReplicas = int32(diutil.CountReadyPods(pods))
		} else {
			job.Status.Allocation = nil
			job.Status.Replicas = 0
			job.Status.ReadyReplicas = 0
		}
	} else {
		if replicas != 0 {
			job.Status.ReadyReplicas = int32(diutil.CountReadyPods(pods))
		}
	}

	if !apiequality.Semantic.DeepEqual(*oldStatus, job.Status) {
		if err := r.ctx.UpdateDIJobStatusInCluster(job); err != nil {
			log.Error(err, "failed to update job status")
			return err
		}
	}
	return nil
}

func (r *DIJobReconciler) reconcileWithJobStatus(job *div1alpha2.DIJob,
	pods []*corev1.Pod) error {
	allocation := job.Status.Allocation
	replicas := int(job.Status.Replicas)
	switch job.Status.Phase {
	case div1alpha2.JobPending:
		if job.Spec.Preemptible && allocation != nil && len(pods) == 0 {
			if err := r.buildAndCreatePods(job, len(allocation), allocation); err != nil {
				return err
			}
			r.ctx.UpdateJobStatus(job, div1alpha2.JobStarting, dicontext.DIJobStartingReason,
				"job is starting since all pods are created.")
		} else if !job.Spec.Preemptible && replicas != 0 && len(pods) == 0 {
			if err := r.buildAndCreatePods(job, replicas, allocation); err != nil {
				return err
			}
			r.ctx.UpdateJobStatus(job, div1alpha2.JobStarting, dicontext.DIJobStartingReason,
				"job is starting since all pods are created.")
		}
	case div1alpha2.JobStarting:
		if diutil.CountPodsScheduled(pods) != replicas && r.ctx.DetectRestart(job, pods, allocation, replicas) {
			r.ctx.UpdateJobStatus(job, div1alpha2.JobRestarting, dicontext.DIJobRestartingReason,
				"job is restarting since conditions changed.")
		} else if len(pods) != replicas {
			r.ctx.UpdateJobStatus(job, div1alpha2.JobRestarting, dicontext.DIJobRestartingReason,
				fmt.Sprintf("job is restarting since the created pods %d are not matched replicas %d.", len(pods), replicas))
		} else if diutil.CountReadyPods(pods) == replicas {
			r.ctx.UpdateJobStatus(job, div1alpha2.JobRunning, dicontext.DIJobRunningReason,
				"job is running since all pods are ready.")
		}
	case div1alpha2.JobRunning:
		if r.ctx.DetectRestart(job, pods, allocation, replicas) || len(pods) != replicas {
			r.ctx.UpdateJobStatus(job, div1alpha2.JobRestarting, dicontext.DIJobRestartingReason,
				fmt.Sprintf("job is restarting since the created pods %d are not matched replicas %d.", len(pods), replicas))
		}
	case div1alpha2.JobRestarting:
		if len(pods) != 0 {
			r.ctx.DeletePods(job, pods)
		} else {
			r.ctx.UpdateJobStatus(job, div1alpha2.JobPending, dicontext.DIJobPendingReason,
				"job is pending since job restarted.")
		}
	}
	return nil
}

func (r *DIJobReconciler) buildAndCreatePods(job *div1alpha2.DIJob, replicas int, allocation []string) error {
	log := r.ctx.Log.WithName("buildAndCreatePods").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	builtPods := []*corev1.Pod{}
	for i := 0; i < replicas; i++ {
		pod := r.ctx.BuildPod(job, i, allocation)
		builtPods = append(builtPods, pod)
	}
	for _, pod := range builtPods {
		err := r.ctx.CreatePod(job, pod)
		if err != nil {
			log.Error(err, "failed to create pod.")
			r.ctx.UpdateJobStatus(job, div1alpha2.JobFailed, dicontext.DIJobFailedReason,
				fmt.Sprintf("job failed since pod %s failed to create.", pod.Name))
			return r.ctx.DeletePods(job, builtPods)
		}
	}
	return nil
}

// we will utilize pod subdomain to reduce the number of services created.
func (r *DIJobReconciler) reconcileServices(job *div1alpha2.DIJob, svcs []*corev1.Service) error {
	if len(svcs) == 0 {
		svc := r.ctx.BuildService(job)
		if err := r.ctx.CreateService(job, svc); err != nil {
			return err
		}
	} else if len(svcs) > 1 {
		for _, svc := range svcs {
			if err := r.ctx.DeleteService(job, svc); err != nil {
				return err
			}
		}
	}
	return nil
}
