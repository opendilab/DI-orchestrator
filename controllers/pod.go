package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div1alpha1 "go-sensephoenix.sensetime.com/di-orchestrator/api/v1alpha1"
	diutil "go-sensephoenix.sensetime.com/di-orchestrator/utils"
)

func (r *DIJobReconciler) reconcilePods(ctx context.Context, job *div1alpha1.DIJob, pods []*corev1.Pod) error {
	log := r.Log.WithValues("dijob", diutil.NamespacedName(job.Namespace, job.Name))

	// classify pods
	collectors, learners, coordinator, ags, _, err := diutil.ClassifyPods(pods)
	if err != nil {
		log.Error(err, "unable to classify pods")
		return err
	}

	// update DIJob status if coordinator and aggregator are created
	if coordinator != nil {
		if err := r.updateDIJobStatus(ctx, job, collectors, learners, coordinator, ags); err != nil {
			return err
		}
	} else {
		// build coordinator pod
		volumes := job.Spec.Volumes
		template := job.Spec.Coordinator.Template.DeepCopy()
		coorpod, coorsvc, coorurl, err := buildPodAndServiceForReplica(template, job, diutil.CoordinatorName,
			diutil.DefaultCoordinatorPort, volumes)
		if err != nil {
			msg := fmt.Sprintf("build coordinator pod for job %s failed", job.Name)
			log.Error(err, msg)
			return err
		}
		// add env
		envs := make(map[string]string)
		envs[diutil.CoordinatorURLEnv] = coorurl
		diutil.SetPodEnv(coorpod, envs)

		if coordinator == nil {
			if err := r.createPodAndService(ctx, job, coorpod, coorsvc); err != nil {
				return err
			}
		}

		// update job status
		msg := fmt.Sprintf("DIJob %s created", job.Name)
		if err := r.updateJobPhase(ctx, job, div1alpha1.JobCreated, DIJobCreatedReason, msg); err != nil {
			return err
		}
	}
	return nil
}

func (r *DIJobReconciler) createPodAndService(ctx context.Context, job *div1alpha1.DIJob, pod *corev1.Pod, svc *corev1.Service) error {
	log := r.Log.WithValues("dijob", diutil.NamespacedName(job.Namespace, job.Name))

	log.Info("create pod ", "pod name:", pod)
	if err := r.createPod(ctx, job, pod); err != nil {
		return err
	}

	log.Info("create service ", "service name:", svc)
	if err := r.createService(ctx, job, svc); err != nil {
		return err
	}
	return nil
}

func (r *DIJobReconciler) createPod(ctx context.Context, job *div1alpha1.DIJob, pod *corev1.Pod) error {
	if err := r.Create(ctx, pod, &client.CreateOptions{}); err != nil {
		msg := fmt.Sprintf("Failed to create pod: %s error: %v", pod.Name, err)
		r.Recorder.Eventf(job, corev1.EventTypeWarning, FailedCreateReason, msg)
		return fmt.Errorf("failed to create pod: %v", err)
	}
	// pod should be the controller of service
	ownRefer := metav1.OwnerReference{
		APIVersion: corev1.SchemeGroupVersion.Version,
		Kind:       "Pod",
		Name:       pod.Name,
		UID:        pod.GetUID(),
		Controller: func(c bool) *bool { return &c }(true),
	}
	for i := range pod.OwnerReferences {
		pod.OwnerReferences[i].Controller = func(c bool) *bool { return &c }(false)
	}
	pod.OwnerReferences = append(pod.OwnerReferences, ownRefer)

	msg := fmt.Sprintf("Create pod: %s", pod.Name)
	r.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulCreateReason, msg)
	return nil
}

func (r *DIJobReconciler) createService(ctx context.Context, job *div1alpha1.DIJob, service *corev1.Service) error {
	if err := r.Create(ctx, service, &client.CreateOptions{}); err != nil {
		msg := fmt.Sprintf("Failed to create service: %s error: %v", service.Name, err)
		r.Recorder.Eventf(job, corev1.EventTypeWarning, FailedCreateReason, msg)
		return fmt.Errorf("failed to create service: %v", err)
	}
	msg := fmt.Sprintf("Create service: %s", service.Name)
	r.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulCreateReason, msg)
	return nil
}

func (r *DIJobReconciler) deletePod(ctx context.Context, job *div1alpha1.DIJob, pod *corev1.Pod) error {
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

func (r *DIJobReconciler) deleteService(ctx context.Context, job *div1alpha1.DIJob, service *corev1.Service) error {
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

func buildPodAndServiceForReplica(template *corev1.PodTemplateSpec, job *div1alpha1.DIJob,
	replicaType string, defaultPort int32, volumes []corev1.Volume) (*corev1.Pod, *corev1.Service, string, error) {
	if string(job.Spec.PriorityClassName) != "" {
		template.Spec.PriorityClassName = string(job.Spec.PriorityClassName)
	}

	// set restart policy for coordinator
	if replicaType == diutil.CoordinatorName && template.Spec.RestartPolicy == "" {
		template.Spec.RestartPolicy = corev1.RestartPolicyNever
	}

	// build owner reference
	ownRefer := metav1.OwnerReference{
		APIVersion: job.APIVersion,
		Kind:       job.Kind,
		Name:       job.Name,
		UID:        job.GetUID(),
		Controller: func(c bool) *bool { return &c }(true),
	}

	// build pod
	pod, port, err := diutil.BuildPodFromTemplate(template, ownRefer, job.Name, job.Namespace, replicaType, defaultPort)
	if err != nil {
		return nil, nil, "", err
	}

	// add env
	envs := make(map[string]string)
	envs[diutil.PodNamespaceEnv] = pod.Namespace
	envs[diutil.PodNameEnv] = pod.Name
	envs[diutil.ServerURLEnv] = diutil.DefaultServerURL
	diutil.SetPodEnv(pod, envs)

	// add volumes
	pod.Spec.Volumes = append(pod.Spec.Volumes, volumes...)

	// build service
	svc := diutil.BuildService(pod.GetLabels(), port)
	svc.SetOwnerReferences([]metav1.OwnerReference{ownRefer})
	svc.Name = pod.Name
	svc.Namespace = pod.Namespace

	// access url
	url := diutil.ConcatURL(svc.Name, svc.Namespace, port)

	return pod, svc, url, nil
}
