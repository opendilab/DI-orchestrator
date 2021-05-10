package controllers

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
)

var (
	statusUpdateRetries = 3
)

const (
	NerveXJobCreatedReason   = "NerveXJobCreated"
	NerveXJobRunningReason   = "NerveXJobRunning"
	NerveXJobFailedReason    = "NerveXJobFailed"
	NerveXJobSucceededReason = "NerveXJobSucceeded"
)

func (r *NerveXJobReconciler) updateNerveXJobStatus(ctx context.Context, job *nervexv1alpha1.NerveXJob) error {
	var err error
	for i := 0; i < statusUpdateRetries; i++ {
		newJob := &nervexv1alpha1.NerveXJob{}
		err := r.Get(ctx, types.NamespacedName{Namespace: job.Namespace, Name: job.Name}, newJob)
		if err != nil {
			break
		}
		newJob.Status = job.Status
		if err := r.Status().Update(ctx, newJob, &client.UpdateOptions{}); err == nil {
			break
		}
	}
	return err
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
	switch pod.Status.Phase {
	case corev1.PodRunning:
		job.Status.ReplicaStatus[replicaType].Active++
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
