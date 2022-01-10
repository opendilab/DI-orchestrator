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
	log := r.ctx.Log.WithValues("dijob", diutil.NamespacedName(job.Namespace, job.Name))
	oldStatus := job.Status.DeepCopy()
	allocation := job.Status.Allocation
	replicas := job.Status.Replicas
	if r.ctx.CheckJobCompletion(job, pods) {
		log.Info("job completed:", job.Name, job.Status.Phase)
		if !apiequality.Semantic.DeepEqual(*oldStatus, job.Status) {
			if err := r.ctx.UpdateDIJobStatusInCluster(job); err != nil {
				log.Error(err, "failed to update DIJobStatus", "job", job.Name)
				return err
			}
		}
		return nil
	}

	if err := r.reconcileWithJobStatus(ctx, job, pods); err != nil {
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
			log.Error(err, "failed to update DIJobStatus", "job", job.Name)
			return err
		}
	}
	return nil
}

func (r *DIJobReconciler) reconcileWithJobStatus(ctx context.Context, job *div1alpha2.DIJob,
	pods []*corev1.Pod) error {
	allocation := job.Status.Allocation
	replicas := int(job.Status.Replicas)
	switch job.Status.Phase {
	case div1alpha2.JobPending:
		if job.Spec.Preemptible {
			if allocation != nil && len(pods) == 0 {
				r.ctx.UpdateJobStatus(job, div1alpha2.JobStarting, dicontext.DIJobStartingReason, fmt.Sprintf("DIJob %s is starting now.", job.Name))
			}
		} else {
			if replicas != 0 && len(pods) == 0 {
				r.ctx.UpdateJobStatus(job, div1alpha2.JobStarting, dicontext.DIJobStartingReason, fmt.Sprintf("DIJob %s is starting now.", job.Name))
			}
		}
	case div1alpha2.JobStarting:
		if job.Spec.Preemptible {
			if (diutil.CountPodsScheduled(pods) != replicas && r.ctx.DetectRestart(job, pods, allocation, replicas)) || allocation == nil {
				r.ctx.UpdateJobStatus(job, div1alpha2.JobRestarting, dicontext.DIJobRestartingReason,
					fmt.Sprintf("DIJob %s is restarting since conditions changed.", job.Name))
			} else if allocation != nil && len(pods) == 0 {
				if err := r.buildAndCreatePods(job, replicas, allocation); err != nil {
					return err
				}
			} else if len(pods) != replicas {
				r.ctx.UpdateJobStatus(job, div1alpha2.JobRestarting, dicontext.DIJobRestartingReason,
					fmt.Sprintf("DIJob %s is restarting since the created pods %d are not matched replicas %d.", job.Name, len(pods), replicas))
			} else if diutil.CountReadyPods(pods) == replicas {
				r.ctx.UpdateJobStatus(job, div1alpha2.JobRunning, dicontext.DIJobRunningReason,
					fmt.Sprintf("DIJob %s is running since all pods are ready.", job.Name))
			}
		} else {
			if diutil.CountPodsScheduled(pods) != replicas && r.ctx.DetectRestart(job, pods, allocation, replicas) {
				r.ctx.UpdateJobStatus(job, div1alpha2.JobRestarting, dicontext.DIJobRestartingReason,
					fmt.Sprintf("DIJob %s is restarting since conditions changed.", job.Name))
			} else if replicas != 0 && len(pods) == 0 {
				if err := r.buildAndCreatePods(job, replicas, allocation); err != nil {
					return err
				}
			} else if len(pods) != replicas {
				r.ctx.UpdateJobStatus(job, div1alpha2.JobRestarting, dicontext.DIJobRestartingReason,
					fmt.Sprintf("DIJob %s is restarting since the created pods %d are not matched replicas %d.", job.Name, len(pods), replicas))
			} else if diutil.CountReadyPods(pods) == replicas {
				r.ctx.UpdateJobStatus(job, div1alpha2.JobRunning, dicontext.DIJobRunningReason,
					fmt.Sprintf("DIJob %s is running since all pods are ready.", job.Name))
			}
		}
	case div1alpha2.JobRunning:
		if r.ctx.DetectRestart(job, pods, allocation, replicas) || len(pods) != replicas {
			r.ctx.UpdateJobStatus(job, div1alpha2.JobRestarting, dicontext.DIJobRestartingReason,
				fmt.Sprintf("DIJob %s is restarting since the created pods %d are not matched replicas %d.", job.Name, len(pods), replicas))
		}
	case div1alpha2.JobRestarting:
		if len(pods) != 0 {
			r.ctx.DeletePods(job, pods)
		} else {
			r.ctx.UpdateJobStatus(job, div1alpha2.JobPending, dicontext.DIJobPendingReason,
				fmt.Sprintf("DIJob %s is pending since job restarted.", job.Name))
		}
	}
	return nil
}

func (r *DIJobReconciler) buildAndCreatePods(job *div1alpha2.DIJob, replicas int, allocation []string) error {
	log := r.ctx.Log.WithName("buildAndCreatePods")
	builtPods := []*corev1.Pod{}
	for i := 0; i < replicas; i++ {
		pod := r.ctx.BuildPod(job, i, allocation)
		builtPods = append(builtPods, pod)
	}
	for _, pod := range builtPods {
		err := r.ctx.CreatePod(job, pod)
		if err != nil {
			log.Error(err, "failed to create pod", "pod", pod.Name)
			r.ctx.UpdateJobStatus(job, div1alpha2.JobFailed, dicontext.DIJobFailedReason,
				fmt.Sprintf("DIJob %s failed because pod %s failed to created.", job.Name, pod.Name))
			return r.ctx.DeletePods(job, builtPods)
		}
	}
	return nil
}
