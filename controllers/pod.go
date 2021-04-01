package controllers

import (
	"context"
	"fmt"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
	nervexutil "go-sensephoenix.sensetime.com/nervex-operator/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *NerveXJobReconciler) reconcilePods(ctx context.Context, job *nervexv1alpha1.NerveXJob, coordinator *corev1.Pod) error {
	if coordinator != nil {
		if err := r.checkPodStatus(ctx, job, coordinator); err != nil {
			return err
		}
	} else {
		if err := r.createPod(ctx, job, nervexutil.CoordinatorName); err != nil {
			return err
		}

		// update job status
		job.Status.Phase = nervexv1alpha1.JobPending
	}
	return nil
}

func (r *NerveXJobReconciler) checkPodStatus(ctx context.Context, job *nervexv1alpha1.NerveXJob, pod *corev1.Pod) error {
	if pod.Status.Phase == corev1.PodRunning {
		job.Status.Phase = nervexv1alpha1.JobRunning
		msg := "pods are ready"
		if err := updateNerveXJobConditions(job, nervexv1alpha1.JobReady, NerveXJobPodsReadyReason, msg); err != nil {
			return err
		}
	} else if pod.Status.Phase == corev1.PodFailed {
		job.Status.Phase = nervexv1alpha1.JobFailed
		if err := updateNerveXJobConditions(job, nervexv1alpha1.JobConditionType(""), "", ""); err != nil {
			return err
		}
	} else if pod.Status.Phase == corev1.PodSucceeded {
		job.Status.Phase = nervexv1alpha1.JobSucceeded
		if err := updateNerveXJobConditions(job, nervexv1alpha1.JobConditionType(""), "", ""); err != nil {
			return err
		}
	}
	return nil
}

func (r *NerveXJobReconciler) createPod(ctx context.Context, job *nervexv1alpha1.NerveXJob, replicaType string) error {
	log := r.Log.WithValues("nervexjob", job.Namespace)
	podTemplate := job.Spec.Coordinator.Template.DeepCopy()

	if podTemplate.Name == "" {
		podTemplate.Name = fmt.Sprintf("%s-%s", job.Name, "coordinator")
	}
	if podTemplate.Namespace == "" {
		podTemplate.Namespace = job.Namespace
	}
	podTemplate.Spec.PriorityClassName = string(job.Spec.PriorityClassName)
	if podTemplate.Spec.RestartPolicy == "" {
		podTemplate.Spec.RestartPolicy = corev1.RestartPolicyNever
	}

	// add labels
	labels := nervexutil.GenLabels(job.Name)
	labels[nervexutil.ReplicaTypeLabel] = replicaType
	nervexutil.AddLabelsToPodTemplate(podTemplate, labels)

	// get port
	port, ok := nervexutil.GetPortFromPodTemplate(podTemplate, nervexutil.DefaultCoordinatorContainerName, nervexutil.DefaultCoordinatorPortName)
	if !ok {
		port = nervexutil.DefaultCoordinatorPort
		log.Info("no port found, use default port", "port", port)
		nervexutil.SetPodTemplatePort(podTemplate, nervexutil.DefaultCoordinatorContainerName, nervexutil.DefaultCoordinatorPortName, port)
	}

	// add env
	envs := make(map[string]string)
	envs["KUBERNETES_POD_NAMESPACE"] = podTemplate.Namespace
	envs["KUBERNETES_POD_NAME"] = podTemplate.Name
	envs["COORDINATOR_PORT"] = fmt.Sprintf("%d", port)
	nervexutil.SetPodTemplateEnv(podTemplate, envs)

	// set owner reference
	ownRefer := metav1.OwnerReference{
		APIVersion: job.APIVersion,
		Kind:       job.Kind,
		Name:       job.Name,
		UID:        job.GetUID(),
		Controller: func(c bool) *bool { return &c }(true),
	}
	podTemplate.SetOwnerReferences([]metav1.OwnerReference{ownRefer})

	pod := nervexutil.BuildPodFromTemplate(podTemplate)
	log.Info("create pod ", "pod name:", pod)
	if err := r.Create(ctx, pod, &client.CreateOptions{}); err != nil {
		return err
	}
	return nil
}
