package context

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

const (
	SuccessfulDeleteReason = "SuccessfulDelete"
	FailedDeleteReason     = "FailedDelete"
	SuccessfulCreateReason = "SuccessfulCreate"
	FailedCreateReason     = "FailedCreate"

	DIJobPendingReason      = "JobPending"
	DIJobStartingReason     = "JobStarting"
	DIJobRunningReason      = "JobRunning"
	DIJobRestartingReason   = "JobRestarting"
	DIJobReschedulingReason = "JobRescheduling"
	DIJobFailedReason       = "JobFailed"
	DIJobSucceededReason    = "JobSucceeded"

	updateTimeout        = 1 * time.Second
	updatedPauseDuration = 50 * time.Millisecond
)

func (c *Context) MarkIncorrectJobFailed(ctx context.Context, obj client.Object) {
	log := c.Log.WithName("MarkIncorrectJobFailed").WithValues("job", diutil.NamespacedName(obj.GetNamespace(), obj.GetName()))
	dclient, err := dynamic.NewForConfig(c.config)
	if err != nil {
		log.Error(err, "failed to create dynamic client")
		return
	}

	// build status
	failedConvertDIJob := fmt.Sprintf("failed to convert type %T to v2alpha1.DIJob", obj)
	status := div2alpha1.DIJobStatus{
		Phase: div2alpha1.JobFailed,
		Conditions: []div2alpha1.JobCondition{
			{
				Type:    div2alpha1.JobFailed,
				Status:  corev1.ConditionTrue,
				Message: failedConvertDIJob,
			},
		},
	}
	statusMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&status)
	if err != nil {
		log.Error(err, "failed to convert status to unstructured")
		return
	}

	// get dijob
	dijobRes := schema.GroupVersionResource{
		Group:    div2alpha1.GroupVersion.Group,
		Version:  div2alpha1.GroupVersion.Version,
		Resource: "dijobs",
	}
	un, err := dclient.Resource(dijobRes).Namespace(obj.GetNamespace()).Get(ctx, obj.GetName(), metav1.GetOptions{})
	if err != nil {
		log.Error(err, "failed to get dijob")
	}
	// set and update status
	unstructured.SetNestedField(un.Object, statusMap, "status")
	var updateErr error
	wait.Poll(updatedPauseDuration, updateTimeout, func() (bool, error) {
		_, updateErr = dclient.Resource(dijobRes).Namespace(obj.GetNamespace()).UpdateStatus(ctx, un, metav1.UpdateOptions{})
		if updateErr == nil {
			return true, nil
		}
		log.Error(updateErr, "failed to update job status")
		return false, nil
	})
}

func (c *Context) UpdateJobRestartsAndReschedulesInCluster(ctx context.Context, old, new *div2alpha1.DIJob) error {
	log := c.Log.WithName("UpdateJobRestartsAndReschedules").WithValues("job", diutil.NamespacedName(new.Namespace, new.Name))
	if old.Status.Restarts == new.Status.Restarts && old.Status.Reschedules == new.Status.Reschedules {
		return nil
	}
	log.Info("update job restarts and reschedules",
		"restarts", fmt.Sprintf("%d->%d", old.Status.Restarts, new.Status.Restarts),
		"reschedules", fmt.Sprintf("%d->%d", old.Status.Reschedules, new.Status.Reschedules))

	err := wait.Poll(updatedPauseDuration, updateTimeout, func() (bool, error) {
		newJob := &div2alpha1.DIJob{}
		err := c.Get(ctx, types.NamespacedName{Namespace: new.Namespace, Name: new.Name}, newJob)
		if err != nil {
			return false, nil
		}
		newJob.Status.Restarts = new.Status.DeepCopy().Restarts
		newJob.Status.Reschedules = new.Status.DeepCopy().Reschedules
		err = c.Status().Update(ctx, newJob, &client.UpdateOptions{})
		if err == nil {
			return true, nil
		}
		return false, nil
	})
	return err
}

func (c *Context) UpdateJobPhaseAndConditionsInCluster(ctx context.Context, old, new *div2alpha1.DIJob) error {
	log := c.Log.WithName("UpdateJobPhaseAndConditions").WithValues("job", diutil.NamespacedName(new.Namespace, new.Name))
	if old.Status.Phase == new.Status.Phase &&
		apiequality.Semantic.DeepEqual(old.Status.Conditions, new.Status.Conditions) {
		return nil
	}
	log.Info("update job phase",
		"phase", fmt.Sprintf("%s->%s", old.Status.Phase, new.Status.Phase))

	err := wait.Poll(updatedPauseDuration, updateTimeout, func() (bool, error) {
		newJob := &div2alpha1.DIJob{}
		err := c.Get(ctx, types.NamespacedName{Namespace: new.Namespace, Name: new.Name}, newJob)
		if err != nil {
			return false, nil
		}
		newJob.Status.Phase = new.Status.DeepCopy().Phase
		newJob.Status.Conditions = new.Status.DeepCopy().Conditions
		err = c.Status().Update(ctx, newJob, &client.UpdateOptions{})
		if err == nil {
			return true, nil
		}
		return false, nil
	})
	return err
}

func (c *Context) UpdateJobAllocationInCluster(ctx context.Context, old, new *div2alpha1.DIJob) error {
	log := c.Log.WithName("UpdateJobAllocation").WithValues("job", diutil.NamespacedName(new.Namespace, new.Name))
	if apiequality.Semantic.DeepEqual(old.Status.Allocation, new.Status.Allocation) {
		return nil
	}
	log.Info("update job allocation",
		"allocation", fmt.Sprintf("%v->%v", old.Status.Allocation, new.Status.Allocation))

	err := wait.Poll(updatedPauseDuration, updateTimeout, func() (bool, error) {
		newJob := &div2alpha1.DIJob{}
		err := c.Get(ctx, types.NamespacedName{Namespace: new.Namespace, Name: new.Name}, newJob)
		if err != nil {
			return false, nil
		}
		newJob.Status.Allocation = new.Status.DeepCopy().Allocation
		err = c.Status().Update(ctx, newJob, &client.UpdateOptions{})
		if err == nil {
			return true, nil
		}
		return false, nil
	})
	return err
}

func (c *Context) UpdateJobProfilingsInCluster(ctx context.Context, old, new *div2alpha1.DIJob) error {
	log := c.Log.WithName("UpdateJobProfilings").WithValues("job", diutil.NamespacedName(new.Namespace, new.Name))
	if old.Status.Profilings == new.Status.Profilings {
		return nil
	}
	log.Info("update job profilings",
		"profilings", fmt.Sprintf("%v->%v", old.Status.Profilings, new.Status.Profilings))

	err := wait.Poll(updatedPauseDuration, updateTimeout, func() (bool, error) {
		newJob := &div2alpha1.DIJob{}
		err := c.Get(ctx, types.NamespacedName{Namespace: new.Namespace, Name: new.Name}, newJob)
		if err != nil {
			return false, nil
		}
		newJob.Status.Profilings = new.Status.DeepCopy().Profilings
		err = c.Status().Update(ctx, newJob, &client.UpdateOptions{})
		if err == nil {
			return true, nil
		}
		return false, nil
	})
	return err
}

func (c *Context) UpdateJobReplicaStatusInCluster(ctx context.Context, old, new *div2alpha1.DIJob) error {
	log := c.Log.WithName("UpdateJobReplicaStatus").WithValues("job", diutil.NamespacedName(new.Namespace, new.Name))
	if old.Status.Replicas == new.Status.Replicas && old.Status.ReadyReplicas == new.Status.ReadyReplicas &&
		apiequality.Semantic.DeepEqual(old.Status.TaskStatus, new.Status.TaskStatus) {
		return nil
	}
	log.Info("update job replica status",
		"ready replicas", fmt.Sprintf("%d->%d", old.Status.ReadyReplicas, new.Status.ReadyReplicas),
		"replicas", fmt.Sprintf("%d->%d", old.Status.Replicas, new.Status.Replicas),
		"task status", fmt.Sprintf("%v->%v", old.Status.TaskStatus, new.Status.TaskStatus))

	err := wait.Poll(updatedPauseDuration, updateTimeout, func() (bool, error) {
		newJob := &div2alpha1.DIJob{}
		err := c.Get(ctx, types.NamespacedName{Namespace: new.Namespace, Name: new.Name}, newJob)
		if err != nil {
			return false, nil
		}
		newJob.Status.ReadyReplicas = new.Status.DeepCopy().ReadyReplicas
		newJob.Status.Replicas = new.Status.DeepCopy().Replicas
		newJob.Status.TaskStatus = new.Status.DeepCopy().TaskStatus
		newJob.Status.CompletionTimestamp = new.Status.DeepCopy().CompletionTimestamp
		err = c.Status().Update(ctx, newJob, &client.UpdateOptions{})
		if err == nil {
			return true, nil
		}
		return false, nil
	})
	return err
}

func (c *Context) UpdateJobStatusByBackoffLimit(job *div2alpha1.DIJob, msg string) {
	if job.Spec.BackoffLimit == nil || job.Status.Restarts < *job.Spec.BackoffLimit {
		restartMsg := fmt.Sprintf("job is restarting since %s", msg)
		c.UpdateJobStatus(job, div2alpha1.JobRestarting, DIJobRestartingReason, restartMsg)
	} else {
		failedMsg := fmt.Sprintf("job failed since %s and exceed backoff limit", msg)
		c.UpdateJobStatus(job, div2alpha1.JobFailed, DIJobFailedReason, failedMsg)
	}
}

func (c *Context) UpdateJobStatus(
	job *div2alpha1.DIJob, phase div2alpha1.Phase, reason string, msg string) {
	log := c.Log.WithName("UpdateJobStatus").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	if job.Status.Phase == phase {
		return
	}
	log.Info(msg)
	updateDIJobConditions(job, phase, reason, msg)
	job.Status.Phase = phase
}

func updateDIJobConditions(job *div2alpha1.DIJob, conditionType div2alpha1.Phase, reason, msg string) {
	newCondition := newCondition(conditionType, reason, msg)

	if diutil.IsSucceeded(job) || diutil.IsFailed(job) {
		for i := range job.Status.Conditions {
			if job.Status.Conditions[i].Type == div2alpha1.JobRunning {
				job.Status.Conditions[i].Status = corev1.ConditionFalse
				job.Status.Conditions[i].LastTransitionTime = metav1.Now()
				job.Status.Conditions[i].LastUpdateTime = metav1.Now()
			}
		}
	}
	setCondition(&job.Status, newCondition)
}

func newCondition(conditionType div2alpha1.Phase, reason, msg string) *div2alpha1.JobCondition {
	return &div2alpha1.JobCondition{
		Type:               conditionType,
		Status:             corev1.ConditionTrue,
		Reason:             reason,
		Message:            msg,
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
	}
}

// setCondition sets the condition for the job, skip if the condition is already exists with the same status and reason
func setCondition(status *div2alpha1.DIJobStatus, condition *div2alpha1.JobCondition) {
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

func getCondition(status *div2alpha1.DIJobStatus, conditionType div2alpha1.Phase) *div2alpha1.JobCondition {
	for _, condition := range status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

func filterOutConditions(conditions []div2alpha1.JobCondition, conditionType div2alpha1.Phase) []div2alpha1.JobCondition {
	newConditions := []div2alpha1.JobCondition{}

	for _, condition := range conditions {
		if condition.Type == conditionType {
			continue
		}
		condition.Status = corev1.ConditionFalse

		newConditions = append(newConditions, condition)
	}
	return newConditions
}

func (c *Context) SetDefaultJobNameInCluster(ctx context.Context, old *div2alpha1.DIJob) error {
	log := c.Log.WithName("SetDefaultJobNameInCluster").WithValues("job", diutil.NamespacedName(old.Namespace, old.Name))

	err := wait.Poll(updatedPauseDuration, updateTimeout, func() (bool, error) {
		newJob := &div2alpha1.DIJob{}
		err := c.Get(ctx, types.NamespacedName{Namespace: old.Namespace, Name: old.Name}, newJob)
		if err != nil {
			return false, nil
		}
		for index, task := range newJob.Spec.Tasks {
			if task.Name == "" && task.Type != div2alpha1.TaskTypeNone {
				old.Spec.Tasks[index].Name = string(newJob.Spec.Tasks[index].Type) // update in local env
				newJob.Spec.Tasks[index].Name = string(newJob.Spec.Tasks[index].Type)
				log.Info(fmt.Sprintf("set default job name %s", newJob.Spec.Tasks[index].Type), "name", fmt.Sprintf("none->%s", newJob.Spec.Tasks[index].Type))
			}
		}
		err = c.Update(ctx, newJob, &client.UpdateOptions{})
		if err == nil {
			return true, nil
		}
		return false, nil
	})
	return err
}
