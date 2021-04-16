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

func (r *NerveXJobReconciler) reconcilePods(ctx context.Context, job *nervexv1alpha1.NerveXJob, pods []*corev1.Pod) error {
	log := r.Log.WithValues("nervexjob", nervexutil.NamespacedName(job.Namespace, job.Name))

	// classify pods
	actors, learners, coordinator, ag, err := nervexutil.ClassifyPods(pods)
	if err != nil {
		log.Error(err, "unable to classify pods")
	}

	// update NerveXJob status if coordinator and aggregator are created
	if coordinator != nil && ag != nil {
		if err := r.checkPodsStatus(ctx, job, actors, learners, coordinator, ag); err != nil {
			return err
		}
	} else {
		if coordinator == nil {
			template := job.Spec.Coordinator.Template.DeepCopy()
			if err := r.createPodAndService(ctx, template, job, nervexutil.CoordinatorName,
				nervexutil.DefaultCoordinatorContainerName, nervexutil.DefaultCoordinatorPortName, nervexutil.DefaultCoordinatorPort); err != nil {
				return err
			}
		}

		if ag == nil {
			// get alconfig to fetch aggregator definition
			alconfig := nervexv1alpha1.ActorLearnerConfig{}
			nsn, err := nervexutil.SplitNamespaceName(r.ALConfig)
			if err != nil {
				return err
			}
			err = r.Get(ctx, nsn, &alconfig)
			if err != nil {
				return err
			}

			template := alconfig.Spec.Aggregator.Template.DeepCopy()
			if err := r.createPodAndService(ctx, template, job, nervexutil.AggregatorName,
				nervexutil.DefaultAggregatorContainerName, nervexutil.DefaultAggregatorPortName, nervexutil.DefaultAggregatorPort); err != nil {
				return err
			}
		}

		// update job status
		job.Status.Phase = nervexv1alpha1.JobCreated
		msg := fmt.Sprintf("NerveXJob %s created", job.Name)
		updateNerveXJobConditions(job, nervexv1alpha1.JobCreated, NerveXJobCreatedReason, msg)

	}
	return nil
}

func (r *NerveXJobReconciler) checkPodsStatus(ctx context.Context, job *nervexv1alpha1.NerveXJob,
	actors []*corev1.Pod, learners []*corev1.Pod, coordinator *corev1.Pod, aggregator *corev1.Pod) error {
	// update replica status
	updateReplicasStatues(job, actors, learners, coordinator, aggregator)

	if job.Status.ReplicaStatus[nervexv1alpha1.ReplicaTypeCoordinator].Active > 0 && job.Status.ReplicaStatus[nervexv1alpha1.ReplicaTypeAggregator].Active > 0 {
		job.Status.Phase = nervexv1alpha1.JobRunning
		msg := fmt.Sprintf("coordinator and aggregator of NerveXJob %s are running", job.Name)
		updateNerveXJobConditions(job, nervexv1alpha1.JobRunning, NerveXJobRunningReason, msg)

	} else if job.Status.ReplicaStatus[nervexv1alpha1.ReplicaTypeCoordinator].Failed > 0 {
		job.Status.Phase = nervexv1alpha1.JobFailed
		msg := fmt.Sprintf("NerveXJob %s failed because coordinator failed", job.Name)
		updateNerveXJobConditions(job, nervexv1alpha1.JobFailed, NerveXJobFailedReason, msg)

	} else if job.Status.ReplicaStatus[nervexv1alpha1.ReplicaTypeCoordinator].Succeeded > 0 {
		job.Status.Phase = nervexv1alpha1.JobSucceeded
		msg := fmt.Sprintf("NerveXJob %s succeeded because coordinator succeeded", job.Name)
		updateNerveXJobConditions(job, nervexv1alpha1.JobSucceeded, NerveXJobSucceededReason, msg)

	}
	return nil
}

func (r *NerveXJobReconciler) createPodAndService(ctx context.Context, template *corev1.PodTemplateSpec, job *nervexv1alpha1.NerveXJob,
	replicaType, containerName, portName string, defaultPort int32) error {
	log := r.Log.WithValues("nervexjob", nervexutil.NamespacedName(job.Namespace, job.Name))

	template.Spec.PriorityClassName = string(job.Spec.PriorityClassName)

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

	// build and create pod
	pod, port, err := nervexutil.BuildPodFromTemplate(template, ownRefer, job.Namespace, replicaType,
		containerName, portName, defaultPort)
	if err != nil {
		return err
	}

	// add env
	envs := make(map[string]string)
	envs[nervexutil.PodNamespaceEnv] = pod.Namespace
	envs[nervexutil.PodNameEnv] = pod.Name
	nervexutil.SetPodEnv(pod, envs)

	log.Info("create pod ", "pod name:", pod)
	if err := r.Create(ctx, pod, &client.CreateOptions{}); err != nil {
		log.Error(err, "failed to create pod", "pod name:", pod)
		return err
	}

	// build and create service
	svc := nervexutil.BuildService(pod.GetLabels(), port, portName)
	svc.SetOwnerReferences([]metav1.OwnerReference{ownRefer})
	svc.Name = pod.Name
	svc.Namespace = pod.Namespace

	log.Info("create service ", "service name:", svc)
	if err := r.Create(ctx, svc, &client.CreateOptions{}); err != nil {
		log.Error(err, "failed to create service", "service name:", svc)
		return err
	}
	return nil
}
