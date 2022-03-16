package context

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

const (
	SuccessfulDeleteReason = "SuccessfulDelete"
	FailedDeleteReason     = "FailedDelete"
	SuccessfulCreateReason = "SuccessfulCreate"
	FailedCreateReason     = "FailedCreate"

	DIJobPendingReason    = "JobPending"
	DIJobStartingReason   = "JobStarting"
	DIJobRunningReason    = "JobRunning"
	DIJobRestartingReason = "JobRestarting"
	DIJobFailedReason     = "JobFailed"
	DIJobSucceededReason  = "JobSucceeded"

	statusUpdateRetries        = 3
	statusUpdatedPauseDuration = 50 * time.Millisecond
)

func (c *Context) CheckJobCompletion(job *div2alpha1.DIJob, pods []*corev1.Pod) (completed bool) {
	succeeded, failed := c.checkPodsCompletion(pods, job.Spec.Preemptible)
	completed = false
	if succeeded != 0 && succeeded == len(pods) {
		msg := "job succeeded since all the replicas are succeeded."
		c.UpdateJobStatus(job, div2alpha1.JobSucceeded, DIJobSucceededReason, msg)
		completed = true
	} else if failed != 0 {
		msg := fmt.Sprintf("job failed since %d replicas failed.", failed)
		c.UpdateJobStatus(job, div2alpha1.JobFailed, DIJobFailedReason, msg)
		completed = true
	}
	return
}

func (c *Context) checkPodsCompletion(pods []*corev1.Pod, preemptable bool) (succeeded, failed int) {
	log := c.Log.WithName("checkPodsCompletion")
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
		} else if preemptable && exit143(pod) {
			log.Info(fmt.Sprintf("pod %s is terminated intentionally", pod.Name))
		} else if diutil.IsTerminating(pod) {
			log.Info(fmt.Sprintf("pod %s has been deleted", pod.Name))
		} else {
			failed++
		}
	}

	return succeeded, failed
}

func (c *Context) DetectRestart(job *div2alpha1.DIJob, pods []*corev1.Pod, allocation []string, replicas int) bool {
	log := c.Log.WithName("DetectRestart").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	for _, pod := range pods {
		areplicas, err := strconv.Atoi(pod.Annotations[dicommon.AnnotationReplicas])
		if err != nil {
			log.Error(err, fmt.Sprintf("annotation %s is not a valid number, should mark job as restarting.", pod.Annotations[dicommon.AnnotationReplicas]))
			return true
		}
		rank, err := strconv.Atoi(pod.Annotations[dicommon.AnnotationRank])
		if err != nil {
			log.Error(err, fmt.Sprintf("annotation %s is not a valid number, should mark job as restarting.", pod.Annotations[dicommon.AnnotationRank]))
			return true
		}
		if job.Spec.Preemptible && (areplicas != len(allocation) || pod.Annotations[dicommon.AnnotationNode] != allocation[rank]) {
			return true
		} else if !job.Spec.Preemptible && areplicas != replicas {
			return true
		}
	}
	return false
}

func (c *Context) MarkIncorrectJobFailed(obj client.Object) {
	log := c.Log.WithName("markIncorrectJobFailed").WithValues("job", diutil.NamespacedName(obj.GetNamespace(), obj.GetName()))
	dclient, err := dynamic.NewForConfig(c.config)
	if err != nil {
		log.Error(err, "failed to create dynamic client")
		return
	}

	// build status
	failedConvertDIJob := fmt.Sprintf("failed to convert type %T to v2alpha1.DIJob", obj)
	status := div2alpha1.DIJobStatus{
		Phase: div2alpha1.JobFailed,
		Conditions: []div2alpha1.DIJobCondition{
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
	un, err := dclient.Resource(dijobRes).Namespace(obj.GetNamespace()).Get(context.Background(), obj.GetName(), metav1.GetOptions{})
	if err != nil {
		log.Error(err, "failed to get dijob")
	}
	// set and update status
	unstructured.SetNestedField(un.Object, statusMap, "status")
	var updateErr error
	for i := 0; i < statusUpdateRetries; i++ {
		_, updateErr = dclient.Resource(dijobRes).Namespace(obj.GetNamespace()).UpdateStatus(context.Background(), un, metav1.UpdateOptions{})
		if updateErr == nil {
			break
		}
	}
	if updateErr != nil {
		log.Error(updateErr, "failed to update job status")
	}
}

func (c *Context) UpdateDIJobStatusInCluster(job *div2alpha1.DIJob) error {
	var err error
	for i := 0; i < statusUpdateRetries; i++ {
		newJob := &div2alpha1.DIJob{}
		err := c.Get(c.ctx, types.NamespacedName{Namespace: job.Namespace, Name: job.Name}, newJob)
		if err != nil {
			break
		}
		newJob.Status = job.Status
		if err := c.Status().Update(c.ctx, newJob, &client.UpdateOptions{}); err == nil {
			time.Sleep(statusUpdatedPauseDuration)
			break
		}
	}
	return err
}

func (c *Context) UpdateJobStatus(
	job *div2alpha1.DIJob, phase div2alpha1.Phase, reason string, msg string) {
	log := c.Log.WithName("UpdateJobStatus").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
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

func newCondition(conditionType div2alpha1.Phase, reason, msg string) *div2alpha1.DIJobCondition {
	return &div2alpha1.DIJobCondition{
		Type:               conditionType,
		Status:             corev1.ConditionTrue,
		Reason:             reason,
		Message:            msg,
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
	}
}

// setCondition sets the condition for the job, skip if the condition is already exists with the same status and reason
func setCondition(status *div2alpha1.DIJobStatus, condition *div2alpha1.DIJobCondition) {
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

func getCondition(status *div2alpha1.DIJobStatus, conditionType div2alpha1.Phase) *div2alpha1.DIJobCondition {
	for _, condition := range status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

func filterOutConditions(conditions []div2alpha1.DIJobCondition, conditionType div2alpha1.Phase) []div2alpha1.DIJobCondition {
	newConditions := []div2alpha1.DIJobCondition{}

	for _, condition := range conditions {
		if condition.Type == conditionType {
			continue
		}
		condition.Status = corev1.ConditionFalse

		newConditions = append(newConditions, condition)
	}
	return newConditions
}
