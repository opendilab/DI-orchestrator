package controllers

import (
	"context"
	"fmt"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
	nervexutil "go-sensephoenix.sensetime.com/nervex-operator/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *NerveXJobReconciler) reconcilePods(ctx context.Context, job *nervexv1alpha1.NerveXJob, pods []*corev1.Pod) error {
	log := r.Log.WithValues("nervexjob", nervexutil.NamespacedName(job.Namespace, job.Name))

	// classify pods
	collectors, learners, coordinator, ag, err := nervexutil.ClassifyPods(pods)
	if err != nil {
		log.Error(err, "unable to classify pods")
		return err
	}

	// update NerveXJob status if coordinator and aggregator are created
	if coordinator != nil && ag != nil {
		if err := r.updateNerveXJobStatus(ctx, job, collectors, learners, coordinator, ag); err != nil {
			return err
		}
	} else {
		// get agconfig to fetch aggregator definition
		agconfig := nervexv1alpha1.AggregatorConfig{}
		nsn, err := nervexutil.SplitNamespaceName(r.AGConfig)
		if err != nil {
			return err
		}
		err = r.Get(ctx, nsn, &agconfig)
		if err != nil {
			return err
		}

		// build aggregator pod
		template := agconfig.Spec.Aggregator.Template.DeepCopy()
		agpod, agsvc, agurl, err := buildPodAndServiceForReplica(template, job, nervexutil.AggregatorName,
			nervexutil.DefaultAggregatorContainerName, nervexutil.DefaultAggregatorPortName, nervexutil.DefaultAggregatorPort)
		if err != nil {
			msg := fmt.Sprintf("build aggregator pod for job %s failed", job.Name)
			log.Error(err, msg)
			return err
		}

		// build coordinator pod
		template = job.Spec.Coordinator.Template.DeepCopy()
		coorpod, coorsvc, coorurl, err := buildPodAndServiceForReplica(template, job, nervexutil.CoordinatorName,
			nervexutil.DefaultCoordinatorContainerName, nervexutil.DefaultCoordinatorPortName, nervexutil.DefaultCoordinatorPort)
		if err != nil {
			msg := fmt.Sprintf("build coordinator pod for job %s failed", job.Name)
			log.Error(err, msg)
			return err
		}
		// add env
		envs := make(map[string]string)
		envs[nervexutil.AggregatorURLEnv] = agurl
		envs[nervexutil.CoordinatorURLEnv] = coorurl
		nervexutil.SetPodEnv(coorpod, envs)
		nervexutil.SetPodEnv(agpod, envs)

		if coordinator == nil {
			if err := r.createPodAndService(ctx, job, coorpod, coorsvc); err != nil {
				return err
			}
		}

		if ag == nil {
			if err := r.createPodAndService(ctx, job, agpod, agsvc); err != nil {
				return err
			}
		}

		// update job status
		msg := fmt.Sprintf("NerveXJob %s created", job.Name)
		if err := r.updateJobPhase(ctx, job, nervexv1alpha1.JobCreated, NerveXJobCreatedReason, msg); err != nil {
			return err
		}
	}
	return nil
}

func (r *NerveXJobReconciler) createPodAndService(ctx context.Context, job *nervexv1alpha1.NerveXJob, pod *corev1.Pod, svc *corev1.Service) error {
	log := r.Log.WithValues("nervexjob", nervexutil.NamespacedName(job.Namespace, job.Name))

	log.Info("create pod ", "pod name:", pod)
	if err := r.createPod(ctx, job, pod); err != nil {
		log.Error(err, "failed to create pod", "pod name:", pod)
		return err
	}

	log.Info("create service ", "service name:", svc)
	if err := r.createService(ctx, job, svc); err != nil {
		log.Error(err, "failed to create service", "service name:", svc)
		return err
	}
	return nil
}

func (r *NerveXJobReconciler) createPod(ctx context.Context, job *nervexv1alpha1.NerveXJob, pod *corev1.Pod) error {

	if err := r.Create(ctx, pod, &client.CreateOptions{}); err != nil {
		msg := fmt.Sprintf("Failed to create pod: %s error: %v", pod.Name, err)
		r.Recorder.Eventf(job, corev1.EventTypeWarning, FailedCreateReason, msg)
		return err
	}
	msg := fmt.Sprintf("Create pod: %s", pod.Name)
	r.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulCreateReason, msg)
	return nil
}

func (r *NerveXJobReconciler) createService(ctx context.Context, job *nervexv1alpha1.NerveXJob, service *corev1.Service) error {

	if err := r.Create(ctx, service, &client.CreateOptions{}); err != nil {
		msg := fmt.Sprintf("Failed to create service: %s error: %v", service.Name, err)
		r.Recorder.Eventf(job, corev1.EventTypeWarning, FailedCreateReason, msg)
		return err
	}
	msg := fmt.Sprintf("Create service: %s", service.Name)
	r.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulCreateReason, msg)
	return nil
}

func (r *NerveXJobReconciler) deletePod(ctx context.Context, job *nervexv1alpha1.NerveXJob, pod *corev1.Pod) error {
	if err := r.Delete(ctx, pod, &client.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
		msg := fmt.Sprintf("Failed to delete pod: %s error: %v", pod.Name, err)
		r.Recorder.Eventf(job, corev1.EventTypeWarning, FailedDeleteReason, msg)
		return err
	}
	msg := fmt.Sprintf("Delete pod: %s", pod.Name)
	r.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulDeleteReason, msg)
	return nil
}

func (r *NerveXJobReconciler) deleteService(ctx context.Context, job *nervexv1alpha1.NerveXJob, service *corev1.Service) error {
	if err := r.Delete(ctx, service, &client.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
		msg := fmt.Sprintf("Failed to delete service: %s error: %v", service.Name, err)
		r.Recorder.Eventf(job, corev1.EventTypeWarning, FailedDeleteReason, msg)
		return err
	}
	msg := fmt.Sprintf("Delete service: %s", service.Name)
	r.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulDeleteReason, msg)
	return nil
}

func buildPodAndServiceForReplica(template *corev1.PodTemplateSpec, job *nervexv1alpha1.NerveXJob,
	replicaType, containerName, portName string, defaultPort int32) (*corev1.Pod, *corev1.Service, string, error) {
	if string(job.Spec.PriorityClassName) != "" {
		template.Spec.PriorityClassName = string(job.Spec.PriorityClassName)
	}

	// set restart policy for coordinator
	if replicaType == nervexutil.CoordinatorName && template.Spec.RestartPolicy == "" {
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
	pod, port, err := nervexutil.BuildPodFromTemplate(template, ownRefer, job.Namespace, replicaType,
		containerName, portName, defaultPort)
	if err != nil {
		return nil, nil, "", err
	}

	// add env
	envs := make(map[string]string)
	envs[nervexutil.PodNamespaceEnv] = pod.Namespace
	envs[nervexutil.PodNameEnv] = pod.Name
	nervexutil.SetPodEnv(pod, envs)

	// build service
	svc := nervexutil.BuildService(pod.GetLabels(), port, portName)
	svc.SetOwnerReferences([]metav1.OwnerReference{ownRefer})
	svc.Name = pod.Name
	svc.Namespace = pod.Namespace

	// access url
	url := nervexutil.ConcatURL(pod.Name, pod.Namespace, port)

	return pod, svc, url, nil
}
