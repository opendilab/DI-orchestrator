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
	NerveXJobPodsReadyReason    = "PodsReady"
	NerveXJobPodsNotReadyReason = "PodsNotReady"
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

func updateNerveXJobConditions(job *nervexv1alpha1.NerveXJob, conditionType nervexv1alpha1.JobConditionType, reason, msg string) error {
	newCondition := newCondition(conditionType, reason, msg)

	if isSucceeded(job) || isFailed(job) {
		for i := range job.Status.Conditions {
			if job.Status.Conditions[i].Type == nervexv1alpha1.JobReady {
				job.Status.Conditions[i].Status = corev1.ConditionFalse
				job.Status.Conditions[i].LastTransitionTime = metav1.Now()
				job.Status.Conditions[i].LastUpdateTime = metav1.Now()
			}
		}
		return nil
	}
	setCondition(&job.Status, newCondition)
	return nil
}

func newCondition(conditionType nervexv1alpha1.JobConditionType, reason, msg string) *nervexv1alpha1.NerveXJobCondition {
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

func getCondition(status *nervexv1alpha1.NerveXJobStatus, conditionType nervexv1alpha1.JobConditionType) *nervexv1alpha1.NerveXJobCondition {
	for _, condition := range status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

func filterOutConditions(conditions []nervexv1alpha1.NerveXJobCondition, conditionType nervexv1alpha1.JobConditionType) []nervexv1alpha1.NerveXJobCondition {
	newConditions := []nervexv1alpha1.NerveXJobCondition{}

	for _, condition := range conditions {
		if condition.Type == conditionType {
			continue
		}

		newConditions = append(newConditions, condition)
	}
	return newConditions
}
