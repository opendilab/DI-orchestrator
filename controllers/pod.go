package controllers

import (
	"context"
	"fmt"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
	nervexutil "go-sensephoenix.sensetime.com/nervex-operator/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *NervexJobReconciler) reconcilePods(ctx context.Context, job *nervexv1alpha1.NervexJob, coordinator *corev1.Pod) error {
	if coordinator != nil {
		if err := r.checkPodStatus(ctx, coordinator); err != nil {
			return err
		}
	} else {
		if err := r.createPod(ctx, job, nervexutil.CoordinatorName); err != nil {
			return err
		}
	}
	return nil
}

func (r *NervexJobReconciler) checkPodStatus(ctx context.Context, pod *corev1.Pod) error {

	return nil
}

func (r *NervexJobReconciler) createPod(ctx context.Context, job *nervexv1alpha1.NervexJob, replicaType string) error {
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
	envs := make(map[string]string, 0)
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
