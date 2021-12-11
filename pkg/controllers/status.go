package controllers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

const (
	SuccessfulDeleteReason = "SuccessfulDelete"
	FailedDeleteReason     = "FailedDelete"
	SuccessfulCreateReason = "SuccessfulCreate"
	FailedCreateReason     = "FailedCreate"

	DIJobPendingReason    = "DIJobPending"
	DIJobStartingReason   = "DIJobStarting"
	DIJobRunningReason    = "DIJobRunning"
	DIJobRestartingReason = "DIJobRestarting"
	DIJobFailedReason     = "DIJobFailed"
	DIJobSucceededReason  = "DIJobSucceeded"

	statusUpdateRetries        = 3
	statusUpdatedPauseDuration = 50 * time.Millisecond
)

func (r *DIJobReconciler) checkJobCompletion(ctx context.Context, job *div1alpha2.DIJob, pods []*corev1.Pod) (completed bool) {
	succeeded, failed := r.checkPodsCompletion(pods, job.Spec.Preemptible)
	completed = false
	if succeeded == len(pods) {
		r.updateJobStatus(ctx, job, div1alpha2.JobSucceeded, DIJobSucceededReason, "DIJob succeeded since all the replicas are succeeded.")
		completed = true
	} else if failed != 0 {
		r.updateJobStatus(ctx, job, div1alpha2.JobFailed, DIJobFailedReason, fmt.Sprintf("DIJob failed since %s replicas failed.", failed))
		completed = true
	}
	return
}

func (r *DIJobReconciler) checkPodsCompletion(pods []*corev1.Pod, preemptable bool) (succeeded, failed int) {
	log := r.Log.WithName("checkPodsCompletion")
	exit143 := func(pod *corev1.Pod) bool {
		if pod.Status.ContainerStatuses == nil {
			return false
		}
		for _, status := range pod.Status.ContainerStatuses {
			if status.State.Terminated != nil && status.State.Terminated.ExitCode == 143 {
				return true
			}
		}
		return false
	}

	succeeded = 0
	failed = 0
	for _, pod := range pods {
		replicas, _ := strconv.Atoi(pod.Annotations[dicommon.AnnotationReplicas])
		if replicas == len(pods) && pod.Status.Phase == corev1.PodSucceeded {
			succeeded++
			continue
		}

		if pod.Status.Phase != corev1.PodUnknown && pod.Status.Phase != corev1.PodFailed {
			continue
		}
		if pod.Status.Reason == "UnexpectedAdmissionError" {
			log.Info(fmt.Sprintf("pod %s UnexpectedAdmissionError occurred, message: %s", pod.Name, pod.Status.Message))
		} else if strings.HasPrefix(pod.Status.Reason, "Outof") {
			log.Info(fmt.Sprintf("pod %s is %s on node %s", pod.Name, pod.Status.Reason, pod.Spec.NodeName))
		} else if preemptable && (diutil.IsTerminating(pod) || exit143(pod)) {
			log.Info(fmt.Sprintf("pod %s is terminated intentionally", pod.Name))
		} else {
			failed++
		}
	}

	return succeeded, failed
}

func (r *DIJobReconciler) updateDIJobStatusInCluster(ctx context.Context, job *div1alpha2.DIJob) error {
	var err error
	for i := 0; i < statusUpdateRetries; i++ {
		newJob := &div1alpha2.DIJob{}
		err := r.Get(ctx, types.NamespacedName{Namespace: job.Namespace, Name: job.Name}, newJob)
		if err != nil {
			break
		}
		newJob.Status = job.Status
		if err := r.Status().Update(ctx, newJob, &client.UpdateOptions{}); err == nil {
			time.Sleep(statusUpdatedPauseDuration)
			break
		}
	}
	return err
}

func (r *DIJobReconciler) updateJobStatus(
	ctx context.Context, job *div1alpha2.DIJob, phase div1alpha2.Phase, reason string, msg string) {

	job.Status.Phase = phase
	updateDIJobConditions(job, phase, reason, msg)
	switch phase {
	case div1alpha2.JobPending, div1alpha2.JobStarting, div1alpha2.JobRestarting:
		r.Recorder.Eventf(job, corev1.EventTypeNormal, reason, msg)
	case div1alpha2.JobRunning:
		// ignore events when job is running
	case div1alpha2.JobFailed:
		r.Recorder.Eventf(job, corev1.EventTypeWarning, reason, msg)
	case div1alpha2.JobSucceeded:
		r.Recorder.Eventf(job, corev1.EventTypeNormal, reason, msg)
	default:
		r.Recorder.Eventf(job, corev1.EventTypeNormal, reason, msg)
	}
}

func updateDIJobConditions(job *div1alpha2.DIJob, conditionType div1alpha2.Phase, reason, msg string) {
	newCondition := newCondition(conditionType, reason, msg)

	if isSucceeded(job) || isFailed(job) {
		for i := range job.Status.Conditions {
			if job.Status.Conditions[i].Type == div1alpha2.JobRunning {
				job.Status.Conditions[i].Status = corev1.ConditionFalse
				job.Status.Conditions[i].LastTransitionTime = metav1.Now()
				job.Status.Conditions[i].LastUpdateTime = metav1.Now()
			}
		}
	}
	setCondition(&job.Status, newCondition)
}

func newCondition(conditionType div1alpha2.Phase, reason, msg string) *div1alpha2.DIJobCondition {
	return &div1alpha2.DIJobCondition{
		Type:               conditionType,
		Status:             corev1.ConditionTrue,
		Reason:             reason,
		Message:            msg,
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
	}
}

// setCondition sets the condition for the job, skip if the condition is already exists with the same status and reason
func setCondition(status *div1alpha2.DIJobStatus, condition *div1alpha2.DIJobCondition) {
	currentCondition := getCondition(status, condition.Type)

	if currentCondition != nil && currentCondition.Reason == condition.Reason && currentCondition.Status == condition.Status {
		return
	}

	// don't update LastTransitionTime if the condition status not changed
	if currentCondition != nil && currentCondition.Status == condition.Status {
		condition.LastTransitionTime = currentCondition.LastTransitionTime
	}

	conditions := filterOutConditions(status.Conditions, condition.Type)
	status.Conditions = append(conditions, *condition)
}

func getCondition(status *div1alpha2.DIJobStatus, conditionType div1alpha2.Phase) *div1alpha2.DIJobCondition {
	for _, condition := range status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

func filterOutConditions(conditions []div1alpha2.DIJobCondition, conditionType div1alpha2.Phase) []div1alpha2.DIJobCondition {
	newConditions := []div1alpha2.DIJobCondition{}

	for _, condition := range conditions {
		if condition.Type == conditionType {
			continue
		}

		newConditions = append(newConditions, condition)
	}
	return newConditions
}
