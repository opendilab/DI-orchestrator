package controllers

import (
	"context"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

func (r *DIJobReconciler) buildAndCreatePod(ctx context.Context, job *div1alpha2.DIJob, rank int, allocation []string) (*corev1.Pod, error) {
	group := job.Spec.Group
	generation := int(job.Status.Generation)
	replicas := int(job.Status.Replicas)
	nodeName := ""
	if allocation != nil {
		nodeName = allocation[rank]
	}

	podTemplate := job.Spec.Template.DeepCopy()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: job.Namespace,
			Name:      diutil.GenPodName(job.Name, generation, rank),
		},
		Spec: podTemplate.Spec,
	}

	labels := map[string]string{
		dicommon.LabelOperator: dicommon.OperatorName,
		dicommon.LabelJob:      job.Name,
		dicommon.LabelGroup:    group,
		dicommon.LabelRank:     strconv.Itoa(rank),
		dicommon.LabelPod:      pod.Name,
	}
	diutil.AddLabelsToPod(pod, labels)

	annotations := map[string]string{
		dicommon.AnnotationGeneration: strconv.Itoa(generation),
		dicommon.AnnotationReplicas:   strconv.Itoa(replicas),
		dicommon.AnnotationRank:       strconv.Itoa(rank),
		dicommon.AnnotationNode:       nodeName,
	}
	diutil.AddAnnotationsToPod(pod, annotations)

	pod.OwnerReferences = append(pod.OwnerReferences, diutil.NewOwnerReference(job.APIVersion, job.Kind, job.Name, job.UID, true))
	if nodeName != "" {
		// TODO(liqingping): add node selector to pod
	}
	pod.Spec.RestartPolicy = corev1.RestartPolicyNever

	envs := map[string]string{
		dicommon.ENVJobID:         diutil.NamespacedName(job.Namespace, job.Name),
		dicommon.ENVJobGeneration: strconv.Itoa(generation),
		dicommon.ENVServerURL:     dicommon.GetDIServerURL(),
	}
	diutil.AddEnvsToPod(pod, envs)

	if err := r.createPod(ctx, job, pod); err != nil {
		return pod, err
	}
	return pod, nil
}

func (r *DIJobReconciler) createPod(ctx context.Context, job *div1alpha2.DIJob, pod *corev1.Pod) error {
	if err := r.Create(ctx, pod, &client.CreateOptions{}); err != nil && !errors.IsAlreadyExists(err) {
		msg := fmt.Sprintf("Failed to create pod: %s error: %v", pod.Name, err)
		r.Recorder.Eventf(job, corev1.EventTypeWarning, FailedCreateReason, msg)
		return fmt.Errorf("failed to create pod: %v", err)
	}
	msg := fmt.Sprintf("Create pod: %s", pod.Name)
	r.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulCreateReason, msg)
	return nil
}

func (r *DIJobReconciler) createService(ctx context.Context, job *div1alpha2.DIJob, service *corev1.Service) error {
	if err := r.Create(ctx, service, &client.CreateOptions{}); err != nil && !errors.IsAlreadyExists(err) {
		msg := fmt.Sprintf("Failed to create service: %s error: %v", service.Name, err)
		r.Recorder.Eventf(job, corev1.EventTypeWarning, FailedCreateReason, msg)
		return fmt.Errorf("failed to create service: %v", err)
	}
	msg := fmt.Sprintf("Create service: %s", service.Name)
	r.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulCreateReason, msg)
	return nil
}

func (r *DIJobReconciler) deletePods(ctx context.Context, job *div1alpha2.DIJob, pods []*corev1.Pod) error {
	var err error
	for _, pod := range pods {
		if err1 := r.deletePod(ctx, job, pod); err != nil {
			err = err1
		}
	}
	return err
}

func (r *DIJobReconciler) deletePod(ctx context.Context, job *div1alpha2.DIJob, pod *corev1.Pod) error {
	if err := r.Delete(ctx, pod,
		&client.DeleteOptions{GracePeriodSeconds: func(a int64) *int64 { return &a }(0)}); err != nil && !errors.IsNotFound(err) {
		msg := fmt.Sprintf("Failed to delete pod: %s error: %v", pod.Name, err)
		r.Recorder.Eventf(job, corev1.EventTypeWarning, FailedDeleteReason, msg)
		return fmt.Errorf("failed to delete pod: %v", err)
	}
	msg := fmt.Sprintf("Delete pod: %s", pod.Name)
	r.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulDeleteReason, msg)
	return nil
}

func (r *DIJobReconciler) deleteService(ctx context.Context, job *div1alpha2.DIJob, service *corev1.Service) error {
	if err := r.Delete(ctx, service,
		&client.DeleteOptions{GracePeriodSeconds: func(a int64) *int64 { return &a }(0)}); err != nil && !errors.IsNotFound(err) {
		msg := fmt.Sprintf("Failed to delete service: %s error: %v", service.Name, err)
		r.Recorder.Eventf(job, corev1.EventTypeWarning, FailedDeleteReason, msg)
		return fmt.Errorf("failed to delete service: %v", err)
	}
	msg := fmt.Sprintf("Delete service: %s", service.Name)
	r.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulDeleteReason, msg)
	return nil
}
