package controllers

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div1alpha1 "opendilab.org/di-orchestrator/pkg/api/v1alpha1"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

const (
	SuccessfulDeleteReason = "SuccessfulDelete"
	FailedDeleteReason     = "FailedDelete"
	SuccessfulCreateReason = "SuccessfulCreate"
	FailedCreateReason     = "FailedCreate"

	DIJobCreatedReason   = "DIJobCreated"
	DIJobRunningReason   = "DIJobRunning"
	DIJobFailedReason    = "DIJobFailed"
	DIJobSucceededReason = "DIJobSucceeded"

	statusUpdateRetries        = 3
	statusUpdatedPauseDuration = 50 * time.Millisecond
)

func (r *DIJobReconciler) updateDIJobStatus(ctx context.Context, job *div1alpha1.DIJob,
	collectors []*corev1.Pod, learners []*corev1.Pod, coordinator *corev1.Pod, aggregators []*corev1.Pod) error {
	log := r.Log.WithValues("dijob", diutil.NamespacedName(job.Namespace, job.Name))
	// update replica status
	updateReplicasStatues(job, collectors, learners, coordinator, aggregators)

	if job.Status.ReplicaStatus[div1alpha1.ReplicaTypeCoordinator].Active > 0 {
		msg := fmt.Sprintf("coordinator and aggregator of DIJob %s are running", job.Name)
		if err := r.updateJobPhase(ctx, job, div1alpha1.JobRunning, DIJobRunningReason, msg); err != nil {
			return err
		}

	} else if job.Status.ReplicaStatus[div1alpha1.ReplicaTypeCoordinator].Failed > 0 {
		msg := fmt.Sprintf("DIJob %s failed because coordinator failed", job.Name)
		log.Info(msg)
		if err := r.updateJobPhase(ctx, job, div1alpha1.JobFailed, DIJobFailedReason, msg); err != nil {
			return err
		}

	} else if job.Status.ReplicaStatus[div1alpha1.ReplicaTypeCoordinator].Succeeded > 0 {
		msg := fmt.Sprintf("DIJob %s succeeded because coordinator succeeded", job.Name)
		log.Info(msg)
		if err := r.updateJobPhase(ctx, job, div1alpha1.JobSucceeded, DIJobSucceededReason, msg); err != nil {
			return err
		}
	}
	return nil
}

func (r *DIJobReconciler) updateDIJobStatusInCluster(ctx context.Context, job *div1alpha1.DIJob) error {
	var err error
	for i := 0; i < statusUpdateRetries; i++ {
		newJob := &div1alpha1.DIJob{}
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

func (r *DIJobReconciler) updateJobPhase(
	ctx context.Context, job *div1alpha1.DIJob, phase div1alpha1.Phase, reason string, msg string) error {
	job.Status.Phase = phase
	updateDIJobConditions(job, phase, reason, msg)

	switch phase {
	case div1alpha1.JobCreated:
		r.Recorder.Eventf(job, corev1.EventTypeNormal, reason, msg)
	case div1alpha1.JobRunning:
		// ignore events when job is running
	case div1alpha1.JobFailed:
		r.Recorder.Eventf(job, corev1.EventTypeWarning, reason, msg)
	case div1alpha1.JobSucceeded:
		r.Recorder.Eventf(job, corev1.EventTypeNormal, reason, msg)
	default:
		r.Recorder.Eventf(job, corev1.EventTypeNormal, reason, msg)
	}

	return nil
}

func initializeDIJobReplicaStatus(job *div1alpha1.DIJob) {
	if job.Status.ReplicaStatus == nil {
		job.Status.ReplicaStatus = make(map[div1alpha1.ReplicaType]*div1alpha1.ReplicaStatus)
	}

	for _, replicaType := range []div1alpha1.ReplicaType{div1alpha1.ReplicaTypeCollector,
		div1alpha1.ReplicaTypeLearner, div1alpha1.ReplicaTypeAggregator, div1alpha1.ReplicaTypeCoordinator} {
		job.Status.ReplicaStatus[replicaType] = &div1alpha1.ReplicaStatus{}
	}
}

func updateReplicasStatues(job *div1alpha1.DIJob,
	collectors []*corev1.Pod, learners []*corev1.Pod, coordinator *corev1.Pod, aggregators []*corev1.Pod) {
	// update collector status
	for _, collector := range collectors {
		updateReplicaStatus(collector, job, div1alpha1.ReplicaTypeCollector)
	}

	// update learner status
	for _, learner := range learners {
		updateReplicaStatus(learner, job, div1alpha1.ReplicaTypeLearner)
	}

	// update aggregator
	for _, aggregator := range aggregators {
		updateReplicaStatus(aggregator, job, div1alpha1.ReplicaTypeAggregator)
	}

	// update coordinator
	updateReplicaStatus(coordinator, job, div1alpha1.ReplicaTypeCoordinator)

}

func updateReplicaStatus(pod *corev1.Pod, job *div1alpha1.DIJob, replicaType div1alpha1.ReplicaType) {
	if diutil.IsTerminating(pod) {
		return
	}
	containerName := ""
	switch replicaType {
	case div1alpha1.ReplicaTypeCoordinator:
		containerName = dicommon.CoordinatorName
	case div1alpha1.ReplicaTypeAggregator:
		containerName = dicommon.AggregatorName
	case div1alpha1.ReplicaTypeCollector:
		containerName = dicommon.CollectorName
	case div1alpha1.ReplicaTypeLearner:
		containerName = dicommon.LearnerName
	}
	switch pod.Status.Phase {
	case corev1.PodRunning:
		terminated := false
		for _, status := range pod.Status.ContainerStatuses {
			if status.Name == containerName && status.State.Terminated != nil {
				terminated = true
				switch status.State.Terminated.Reason {
				case "Error":
					job.Status.ReplicaStatus[replicaType].Failed++
				case "Completed":
					job.Status.ReplicaStatus[replicaType].Succeeded++
				}
				break
			}
		}
		if !terminated {
			job.Status.ReplicaStatus[replicaType].Active++
		}
	case corev1.PodFailed:
		job.Status.ReplicaStatus[replicaType].Failed++
	case corev1.PodSucceeded:
		job.Status.ReplicaStatus[replicaType].Succeeded++
	}
}

func updateDIJobConditions(job *div1alpha1.DIJob, conditionType div1alpha1.Phase, reason, msg string) {
	newCondition := newCondition(conditionType, reason, msg)

	if isSucceeded(job) || isFailed(job) {
		for i := range job.Status.Conditions {
			if job.Status.Conditions[i].Type == div1alpha1.JobRunning {
				job.Status.Conditions[i].Status = corev1.ConditionFalse
				job.Status.Conditions[i].LastTransitionTime = metav1.Now()
				job.Status.Conditions[i].LastUpdateTime = metav1.Now()
			}
		}
	}
	setCondition(&job.Status, newCondition)
}

func newCondition(conditionType div1alpha1.Phase, reason, msg string) *div1alpha1.DIJobCondition {
	return &div1alpha1.DIJobCondition{
		Type:               conditionType,
		Status:             corev1.ConditionTrue,
		Reason:             reason,
		Message:            msg,
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
	}
}

// setCondition sets the condition for the job, skip if the condition is already exists with the same status and reason
func setCondition(status *div1alpha1.DIJobStatus, condition *div1alpha1.DIJobCondition) {
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

func getCondition(status *div1alpha1.DIJobStatus, conditionType div1alpha1.Phase) *div1alpha1.DIJobCondition {
	for _, condition := range status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

func filterOutConditions(conditions []div1alpha1.DIJobCondition, conditionType div1alpha1.Phase) []div1alpha1.DIJobCondition {
	newConditions := []div1alpha1.DIJobCondition{}

	for _, condition := range conditions {
		if condition.Type == conditionType {
			continue
		}

		newConditions = append(newConditions, condition)
	}
	return newConditions
}
