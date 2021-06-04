package controllers

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
	nervexutil "go-sensephoenix.sensetime.com/nervex-operator/utils"
)

const (
	SuccessfulDeleteReason = "SuccessfulDelete"
	FailedDeleteReason     = "FailedDelete"
	SuccessfulCreateReason = "SuccessfulCreate"
	FailedCreateReason     = "FailedCreate"

	NerveXJobCreatedReason   = "NerveXJobCreated"
	NerveXJobRunningReason   = "NerveXJobRunning"
	NerveXJobFailedReason    = "NerveXJobFailed"
	NerveXJobSucceededReason = "NerveXJobSucceeded"

	statusUpdateRetries        = 3
	statusUpdatedPauseDuration = 50 * time.Millisecond
)

func (r *NerveXJobReconciler) updateNerveXJobStatus(ctx context.Context, job *nervexv1alpha1.NerveXJob,
	collectors []*corev1.Pod, learners []*corev1.Pod, coordinator *corev1.Pod, aggregator *corev1.Pod) error {
	log := r.Log.WithValues("nervexjob", nervexutil.NamespacedName(job.Namespace, job.Name))
	// update replica status
	updateReplicasStatues(job, collectors, learners, coordinator, aggregator)

	if job.Status.ReplicaStatus[nervexv1alpha1.ReplicaTypeCoordinator].Active > 0 &&
		job.Status.ReplicaStatus[nervexv1alpha1.ReplicaTypeAggregator].Active > 0 {
		msg := fmt.Sprintf("coordinator and aggregator of NerveXJob %s are running", job.Name)
		if err := r.updateJobPhase(ctx, job, nervexv1alpha1.JobRunning, NerveXJobRunningReason, msg); err != nil {
			return err
		}

	} else if job.Status.ReplicaStatus[nervexv1alpha1.ReplicaTypeCoordinator].Failed > 0 {
		msg := fmt.Sprintf("NerveXJob %s failed because coordinator failed", job.Name)
		log.Info(msg)
		if err := r.updateJobPhase(ctx, job, nervexv1alpha1.JobFailed, NerveXJobFailedReason, msg); err != nil {
			return err
		}

	} else if job.Status.ReplicaStatus[nervexv1alpha1.ReplicaTypeCoordinator].Succeeded > 0 {
		msg := fmt.Sprintf("NerveXJob %s succeeded because coordinator succeeded", job.Name)
		log.Info(msg)
		if err := r.updateJobPhase(ctx, job, nervexv1alpha1.JobSucceeded, NerveXJobSucceededReason, msg); err != nil {
			return err
		}
	}
	return nil
}

func (r *NerveXJobReconciler) updateNerveXJobStatusInCluster(ctx context.Context, job *nervexv1alpha1.NerveXJob) error {
	var err error
	for i := 0; i < statusUpdateRetries; i++ {
		newJob := &nervexv1alpha1.NerveXJob{}
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

func (r *NerveXJobReconciler) updateJobPhase(
	ctx context.Context, job *nervexv1alpha1.NerveXJob, phase nervexv1alpha1.Phase, reason string, msg string) error {
	job.Status.Phase = phase
	updateNerveXJobConditions(job, phase, reason, msg)

	switch phase {
	case nervexv1alpha1.JobCreated, nervexv1alpha1.JobRunning:
		// ignore events when job are created or running
	case nervexv1alpha1.JobFailed:
		r.Recorder.Eventf(job, corev1.EventTypeWarning, reason, msg)
	case nervexv1alpha1.JobSucceeded:
		r.Recorder.Eventf(job, corev1.EventTypeNormal, reason, msg)
	default:
		r.Recorder.Eventf(job, corev1.EventTypeNormal, reason, msg)
	}

	return nil
}

func initializeNerveXJobReplicaStatus(job *nervexv1alpha1.NerveXJob) {
	if job.Status.ReplicaStatus == nil {
		job.Status.ReplicaStatus = make(map[nervexv1alpha1.ReplicaType]*nervexv1alpha1.ReplicaStatus)
	}

	for _, replicaType := range []nervexv1alpha1.ReplicaType{nervexv1alpha1.ReplicaTypeCollector,
		nervexv1alpha1.ReplicaTypeLearner, nervexv1alpha1.ReplicaTypeAggregator, nervexv1alpha1.ReplicaTypeCoordinator} {
		job.Status.ReplicaStatus[replicaType] = &nervexv1alpha1.ReplicaStatus{}
	}
}

func updateReplicasStatues(job *nervexv1alpha1.NerveXJob,
	collectors []*corev1.Pod, learners []*corev1.Pod, coordinator *corev1.Pod, aggregator *corev1.Pod) {
	// update collector status
	for _, collector := range collectors {
		updateReplicaStatus(collector, job, nervexv1alpha1.ReplicaTypeCollector)
	}

	// update learner status
	for _, learner := range learners {
		updateReplicaStatus(learner, job, nervexv1alpha1.ReplicaTypeLearner)
	}

	// update aggregator
	updateReplicaStatus(aggregator, job, nervexv1alpha1.ReplicaTypeAggregator)

	// update coordinator
	updateReplicaStatus(coordinator, job, nervexv1alpha1.ReplicaTypeCoordinator)

}

func updateReplicaStatus(pod *corev1.Pod, job *nervexv1alpha1.NerveXJob, replicaType nervexv1alpha1.ReplicaType) {
	if nervexutil.IsTerminating(pod) {
		return
	}
	containerName := ""
	switch replicaType {
	case nervexv1alpha1.ReplicaTypeCoordinator:
		containerName = nervexutil.CoordinatorName
	case nervexv1alpha1.ReplicaTypeAggregator:
		containerName = nervexutil.AggregatorName
	case nervexv1alpha1.ReplicaTypeCollector:
		containerName = nervexutil.CollectorName
	case nervexv1alpha1.ReplicaTypeLearner:
		containerName = nervexutil.LearnerName
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

func updateNerveXJobConditions(job *nervexv1alpha1.NerveXJob, conditionType nervexv1alpha1.Phase, reason, msg string) {
	newCondition := newCondition(conditionType, reason, msg)

	if isSucceeded(job) || isFailed(job) {
		for i := range job.Status.Conditions {
			if job.Status.Conditions[i].Type == nervexv1alpha1.JobRunning {
				job.Status.Conditions[i].Status = corev1.ConditionFalse
				job.Status.Conditions[i].LastTransitionTime = metav1.Now()
				job.Status.Conditions[i].LastUpdateTime = metav1.Now()
			}
		}
	}
	setCondition(&job.Status, newCondition)
}

func newCondition(conditionType nervexv1alpha1.Phase, reason, msg string) *nervexv1alpha1.NerveXJobCondition {
	return &nervexv1alpha1.NerveXJobCondition{
		Type:               conditionType,
		Status:             corev1.ConditionTrue,
		Reason:             reason,
		Message:            msg,
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
	}
}

// setCondition sets the condition for the job, skip if the condition is already exists with the same status and reason
func setCondition(status *nervexv1alpha1.NerveXJobStatus, condition *nervexv1alpha1.NerveXJobCondition) {
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

func getCondition(status *nervexv1alpha1.NerveXJobStatus, conditionType nervexv1alpha1.Phase) *nervexv1alpha1.NerveXJobCondition {
	for _, condition := range status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

func filterOutConditions(conditions []nervexv1alpha1.NerveXJobCondition, conditionType nervexv1alpha1.Phase) []nervexv1alpha1.NerveXJobCondition {
	newConditions := []nervexv1alpha1.NerveXJobCondition{}

	for _, condition := range conditions {
		if condition.Type == conditionType {
			continue
		}

		newConditions = append(newConditions, condition)
	}
	return newConditions
}
