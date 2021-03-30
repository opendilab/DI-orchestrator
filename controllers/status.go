package controllers

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
)

func updateNervexJobConditions(job *nervexv1alpha1.NervexJob, conditionType nervexv1alpha1.JobConditionType, reason, msg string) error {
	newCondition := newCondition(conditionType, reason, msg)

	if isSucceeded(job) || isFailed(job) {
		return nil
	}
	setCondition(&job.Status, newCondition)
	return nil
}

func newCondition(conditionType nervexv1alpha1.JobConditionType, reason, msg string) *nervexv1alpha1.NervexJobCondition {
	return &nervexv1alpha1.NervexJobCondition{
		Type:               conditionType,
		Status:             corev1.ConditionTrue,
		Reason:             reason,
		Message:            msg,
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
	}
}

// setCondition sets the condition for the job, skip if the condition is already exists with the same status and reason
func setCondition(status *nervexv1alpha1.NervexJobStatus, condition *nervexv1alpha1.NervexJobCondition) {
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

func getCondition(status *nervexv1alpha1.NervexJobStatus, conditionType nervexv1alpha1.JobConditionType) *nervexv1alpha1.NervexJobCondition {
	for _, condition := range status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

func filterOutConditions(conditions []nervexv1alpha1.NervexJobCondition, conditionType nervexv1alpha1.JobConditionType) []nervexv1alpha1.NervexJobCondition {
	newConditions := []nervexv1alpha1.NervexJobCondition{}

	for _, condition := range conditions {
		if condition.Type == conditionType {
			continue
		}

		newConditions = append(newConditions, condition)
	}
	return newConditions
}
