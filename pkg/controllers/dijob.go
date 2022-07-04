package controllers

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"

	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
	dicontext "opendilab.org/di-orchestrator/pkg/context"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

func (r *DIJobReconciler) reconcileReplicas(ctx context.Context, job *div2alpha1.DIJob,
	pods []*corev1.Pod, services []*corev1.Service) error {
	log := r.ctx.Log.WithName("reconcileReplicas").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	old := job.DeepCopy()

	// update job status before return
	defer func() {
		if err := r.ctx.UpdateJobPhaseAndConditionsInCluster(ctx, old, job); err != nil {
			log.Error(err, "update job phase and conditions.")
		}
		if err := r.ctx.UpdateJobReplicaStatusInCluster(ctx, old, job); err != nil {
			log.Error(err, "update job replica status.")
		}
		if err := r.ctx.UpdateJobAllocationInCluster(ctx, old, job); err != nil {
			log.Error(err, "update allocation.")
		}
	}()

	if err := r.reconcileWithJobStatus(ctx, job, pods); err != nil {
		return err
	}

	if err := r.reconcileServices(ctx, job, services); err != nil {
		return err
	}
	return nil
}

func (r *DIJobReconciler) reconcileWithJobStatus(ctx context.Context, job *div2alpha1.DIJob,
	totalPods []*corev1.Pod) error {
	log := r.ctx.Log.WithName("reconcileWithJobStatus").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	allocation := job.Status.Allocation
	replicas := 0
	for _, task := range job.Spec.Tasks {
		replicas += int(task.Replicas)
	}

	// update status replicas
	err := r.updateStatusReplicas(job, totalPods, replicas)
	if err != nil {
		return err
	}

	// filter out terminating pods
	filters := make(diutil.Filters, 0)
	filters = append(filters, diutil.NonTerminatingPodFilter)
	pods := diutil.FilterPods(totalPods, filters)

	switch job.Status.Phase {
	case div2alpha1.JobPending:
		if replicas == 0 {
			break
		}
		if len(totalPods) == 0 {
			if err := r.buildAndCreatePods(ctx, job, replicas, allocation); err != nil {
				return err
			}
		} else if len(pods) == replicas {
			r.ctx.UpdateJobStatus(job, div2alpha1.JobStarting, dicontext.DIJobStartingReason,
				"job is starting since all pods are created.")
		} else if len(pods) != len(totalPods) {
			// delete terminating pods
			if err := r.ctx.DeletePods(ctx, job, totalPods); err != nil {
				return err
			}
		}
	case div2alpha1.JobStarting:
		if r.checkPodsCompletion(job, pods) {
			log.Info("job phase changed", "old", div2alpha1.JobStarting, "new", job.Status.Phase)
			return nil
		}
		if diutil.CountScheduledPods(pods) != replicas && r.detectReschedule(job, pods, allocation, replicas) {
			r.ctx.UpdateJobStatus(job, div2alpha1.JobRescheduling, dicontext.DIJobReschedulingReason,
				"job is rescheduling since replicas or allocation changed.")
		} else if diutil.CountReadyPods(pods) == replicas {
			r.ctx.UpdateJobStatus(job, div2alpha1.JobRunning, dicontext.DIJobRunningReason,
				"job is running since all pods are ready.")
		}
	case div2alpha1.JobRunning:
		if r.checkPodsCompletion(job, pods) {
			log.Info("job phase changed.", "old", div2alpha1.JobRunning, "new", job.Status.Phase)
			return nil
		}
		if r.detectReschedule(job, pods, allocation, replicas) {
			r.ctx.UpdateJobStatus(job, div2alpha1.JobRescheduling, dicontext.DIJobReschedulingReason,
				fmt.Sprintf("job is rescheduling since replicas changed, replicas: %d, allocation: %v.", replicas, allocation))
		}
	case div2alpha1.JobRestarting:
		if len(pods) > replicas {
			if err := r.deleteRedundantReplicas(ctx, job, pods, replicas); err != nil {
				return err
			}
		} else if len(pods) < replicas {
			if err := r.createMissedReplicas(ctx, job, pods, replicas, allocation); err != nil {
				return err
			}
		} else {
			allReady, err := r.deleteFailedReplicas(ctx, job, pods)
			if err != nil {
				return err
			}
			if allReady {
				r.ctx.UpdateJobStatus(job, div2alpha1.JobStarting, dicontext.DIJobStartingReason,
					fmt.Sprintf("job is starting since the created pods %d are matched replicas %d.", len(pods), replicas))
			}
		}
	case div2alpha1.JobRescheduling:
		if job.Spec.Preemptible {
			if len(pods) != 0 {
				if err := r.ctx.DeletePods(ctx, job, pods); err != nil {
					return err
				}
			} else {
				if err := r.buildAndCreatePods(ctx, job, replicas, allocation); err != nil {
					return err
				}
				r.ctx.UpdateJobStatus(job, div2alpha1.JobStarting, dicontext.DIJobStartingReason,
					"job is starting since all pods are created.")
			}
		} else {
			if len(pods) > replicas {
				if err := r.deleteRedundantReplicas(ctx, job, pods, replicas); err != nil {
					return err
				}
			} else if len(pods) < replicas {
				// delete terminating pods
				filters := make(diutil.Filters, 0)
				filters = append(filters, diutil.TerminatingPodFilter)
				terminatingPods := diutil.FilterPods(totalPods, filters)
				if err := r.ctx.DeletePods(ctx, job, terminatingPods); err != nil {
					return err
				}

				// create missed replicas
				if err := r.createMissedReplicas(ctx, job, pods, replicas, allocation); err != nil {
					return err
				}
			} else {
				if replicas == 0 {
					r.ctx.UpdateJobStatus(job, div2alpha1.JobPending, dicontext.DIJobPendingReason,
						"job is pending since no replicas assigned.")
				} else {
					r.ctx.UpdateJobStatus(job, div2alpha1.JobStarting, dicontext.DIJobStartingReason,
						fmt.Sprintf("job is starting since the created pods %d are matched replicas %d.", len(pods), replicas))
				}
			}
		}
	}
	return nil
}

func (r *DIJobReconciler) buildAndCreatePods(ctx context.Context, job *div2alpha1.DIJob, replicas int, allocation []string) error {
	log := r.ctx.Log.WithName("buildAndCreatePods").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	builtPods := []*corev1.Pod{}
	podRank := 0
	for taskNum, task := range job.Spec.Tasks {
		for i := 0; i < int(task.Replicas); i++ {
			pod := r.ctx.BuildPod(job, podRank, allocation, taskNum, i)
			builtPods = append(builtPods, pod)
			podRank++
		}
	}
	for _, pod := range builtPods {
		err := r.ctx.CreatePod(ctx, job, pod)
		if err != nil {
			log.Error(err, "failed to create pod.")
			r.ctx.UpdateJobStatusByBackoffLimit(job,
				fmt.Sprintf("pod %s failed to create.", pod.Name))
			return r.ctx.DeletePods(ctx, job, builtPods)
		}
	}
	return nil
}

// we will utilize pod subdomain to reduce the number of services created.
func (r *DIJobReconciler) reconcileServices(ctx context.Context, job *div2alpha1.DIJob, svcs []*corev1.Service) error {
	if len(svcs) == 0 {
		svc := r.ctx.BuildService(job)
		if err := r.ctx.CreateService(ctx, job, svc); err != nil {
			return err
		}
	} else if len(svcs) > 1 {
		for _, svc := range svcs {
			if err := r.ctx.DeleteService(ctx, job, svc); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *DIJobReconciler) checkPodsCompletion(job *div2alpha1.DIJob, pods []*corev1.Pod) bool {
	return r.checkAllPodsSucceeded(job, pods) || r.checkPodsFailed(job, pods)
}

func (r *DIJobReconciler) checkAllPodsSucceeded(job *div2alpha1.DIJob, pods []*corev1.Pod) bool {
	succeeded, _ := diutil.CountCompletedPods(pods, job.Spec.Preemptible)
	if succeeded != 0 && succeeded == len(pods) {
		msg := "job succeeded since all the replicas are succeeded."
		r.ctx.UpdateJobStatus(job, div2alpha1.JobSucceeded, dicontext.DIJobSucceededReason, msg)
		return true
	}
	return false
}

func (r *DIJobReconciler) checkPodsFailed(job *div2alpha1.DIJob, pods []*corev1.Pod) bool {
	_, failed := diutil.CountCompletedPods(pods, job.Spec.Preemptible)
	if failed != 0 {
		r.ctx.UpdateJobStatusByBackoffLimit(job,
			fmt.Sprintf("%d pods failed.", failed))
		return true
	}
	return false
}

func (r *DIJobReconciler) detectReschedule(job *div2alpha1.DIJob, pods []*corev1.Pod, allocation []string, replicas int) bool {
	log := r.ctx.Log.WithName("detectReschedule").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	if !job.Spec.Preemptible {
		return len(pods) != replicas
	}

	for _, pod := range pods {
		areplicas, err := strconv.Atoi(pod.Annotations[dicommon.AnnotationReplicas])
		if err != nil {
			log.Error(err, fmt.Sprintf("annotation %s is not a valid number, job is rescheduled.", pod.Annotations[dicommon.AnnotationReplicas]))
			return true
		}
		rank, err := strconv.Atoi(pod.Annotations[dicommon.AnnotationRank])
		if err != nil {
			log.Error(err, fmt.Sprintf("annotation %s is not a valid number, job is rescheduled.", pod.Annotations[dicommon.AnnotationRank]))
			return true
		}

		if areplicas != len(allocation) || pod.Annotations[dicommon.AnnotationNode] != allocation[rank] {
			return true
		}
	}
	return false
}

func (r *DIJobReconciler) deleteRedundantReplicas(ctx context.Context, job *div2alpha1.DIJob, pods []*corev1.Pod, replicas int) error {
	//log := r.ctx.Log.WithName("deleteRedundantReplicas").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	typeNumMap := map[string]int{} // record each task's required replicas.
	for _, task := range job.Spec.Tasks {
		typeNumMap[string(task.Type)] = int(task.Replicas)
	}
	for _, pod := range pods {
		taskType := pod.Labels[dicommon.LabelTaskType]
		if typeNumMap[taskType] > 0 {
			typeNumMap[taskType]-- // upper limit was not reached.
			continue
		}
		if err := r.ctx.DeletePod(ctx, job, pod); err != nil { // this pod is redundant, because there is no replica for it.
			return err
		}
	}

	return nil
}

func (r *DIJobReconciler) createMissedReplicas(ctx context.Context, job *div2alpha1.DIJob, pods []*corev1.Pod, replicas int, allocation []string) error {
	log := r.ctx.Log.WithName("createMissedReplicas").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	nextGlobalRank := -1
	for index, task := range job.Spec.Tasks {
		localTrace := make([]bool, task.Replicas)
		for _, pod := range pods {
			if pod.Labels[dicommon.LabelTaskType] != string(task.Type) {
				continue
			}
			localRank, err := strconv.Atoi(pod.Annotations[dicommon.AnnotationTaskRank])
			if err != nil {
				log.Error(err, fmt.Sprintf("annotation %s is not a valid number, should mark job as restarting.",
					pod.Annotations[dicommon.AnnotationTaskRank]))
				return err
			}
			localTrace[localRank] = true
		}
		for localRank := 0; localRank < int(task.Replicas); localRank++ {
			if localTrace[localRank] {
				continue
			}
			if nextGlobalRank == -1 { // only run once
				curGlobalRank, err := r.findMaxGlobalRank(job, pods)
				if err != nil {
					log.Error(err, fmt.Sprintf("can not find a suitable global rank number, task type is %s, local rank is %d.", task.Type, localRank))
					return err
				}
				nextGlobalRank = curGlobalRank + 1
			}

			pod := r.ctx.BuildPod(job, nextGlobalRank, allocation, index, localRank)
			if err := r.ctx.CreatePod(ctx, job, pod); err != nil {
				return err
			}
			nextGlobalRank++
		}
	}
	return nil
}
func (r *DIJobReconciler) findMaxGlobalRank(job *div2alpha1.DIJob, pods []*corev1.Pod) (int, error) {
	log := r.ctx.Log.WithName("findMaxGlobalRank").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	maxGlobalRank := -1
	for _, pod := range pods {
		globalRank, err := strconv.Atoi(pod.Annotations[dicommon.AnnotationRank])
		if err != nil {
			log.Error(err, fmt.Sprintf("annotation %s is not a valid number, should mark job as restarting.",
				pod.Annotations[dicommon.AnnotationRank]))
			return -1, err
		}
		if globalRank > maxGlobalRank {
			maxGlobalRank = globalRank
		}
	}
	if maxGlobalRank < 0 {
		err := errors.New("max global rank is a invalid number")
		return -1, err
	}
	return maxGlobalRank, nil
}

func (r *DIJobReconciler) deleteFailedReplicas(ctx context.Context, job *div2alpha1.DIJob, pods []*corev1.Pod) (bool, error) {
	allReady := true
	for _, pod := range pods {
		if diutil.IsPodFailed(pod, job.Spec.Preemptible) {
			allReady = false
			if err := r.ctx.DeletePod(ctx, job, pod); err != nil {
				return false, err
			}
		}
	}
	return allReady, nil
}

func (r *DIJobReconciler) updateStatusReplicas(job *div2alpha1.DIJob, pods []*corev1.Pod, replicas int) error {
	allocation := job.Status.Allocation
	if job.Spec.Preemptible {
		if len(allocation) != 0 {
			job.Status.Replicas = int32(len(allocation))
			job.Status.ReadyReplicas = int32(diutil.CountReadyPods(pods))
		} else {
			job.Status.Allocation = nil
			job.Status.Replicas = 0
			job.Status.ReadyReplicas = 0
		}
	} else {
		job.Status.Replicas = int32(replicas)
		job.Status.ReadyReplicas = int32(diutil.CountReadyPods(pods))

		// update task status
		tasksStatus := make(map[string]div2alpha1.TaskStatus)
		taskPods := diutil.SplitTypedPods(pods)
		for taskName, tmppods := range taskPods {
			taskStatus := make(div2alpha1.TaskStatus)
			for _, pod := range tmppods {
				taskStatus[pod.Status.Phase]++
			}
			tasksStatus[taskName] = taskStatus
		}
		job.Status.TaskStatus = tasksStatus
	}
	return nil
}
