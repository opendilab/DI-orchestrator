package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div1alpha1 "opendilab.org/di-orchestrator/api/v1alpha1"
	dicommon "opendilab.org/di-orchestrator/common"
	diutil "opendilab.org/di-orchestrator/utils"
)

func isSucceeded(job *div1alpha1.DIJob) bool {
	return job.Status.Phase == div1alpha1.JobSucceeded
}

func isFailed(job *div1alpha1.DIJob) bool {
	return job.Status.Phase == div1alpha1.JobFailed
}

func (r *DIJobReconciler) reconcileReplicas(ctx context.Context, job *div1alpha1.DIJob, pods []*corev1.Pod, services []*corev1.Service) error {
	log := r.Log.WithValues("dijob", diutil.NamespacedName(job.Namespace, job.Name))

	if err := r.reconcilePodsAndServices(ctx, job, pods, services); err != nil {
		log.Error(err, "failed to reconcile pods and services")
		return err
	}

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
		coorpod, coorsvc, coorurl, err := buildPodAndServiceForReplica(template, job, dicommon.CoordinatorName, volumes)
		if err != nil {
			msg := fmt.Sprintf("build coordinator pod for job %s failed", job.Name)
			log.Error(err, msg)
			return err
		}
		// add env
		envs := make(map[string]string)
		envs[dicommon.CoordinatorURLEnv] = coorurl
		diutil.AddEnvsToPod(coorpod, envs)

		if err := r.createPodAndService(ctx, job, coorpod, coorsvc); err != nil {
			return err
		}
	}
	return nil
}

// addDIJob is the event handler responsible for handling job add events
func (r *DIJobReconciler) addDIJob(obj client.Object) {
	log := r.Log.WithValues("dijob", diutil.NamespacedName(obj.GetNamespace(), obj.GetName()))
	job, ok := obj.(*div1alpha1.DIJob)
	if !ok {
		log.Error(fmt.Errorf("failed to convert object DIJob: %s/%s", obj.GetNamespace(), obj.GetName()), "")
		r.markIncorrectJobFailed(obj)
		return
	}

	oldStatus := job.Status.DeepCopy()
	// update job status
	msg := fmt.Sprintf("DIJob %s created", job.Name)
	if err := r.updateJobPhase(context.Background(), job, div1alpha1.JobCreated, DIJobCreatedReason, msg); err != nil {
		log.Error(err, "failed to update job status")
		return
	}

	log.Info(fmt.Sprintf("DIJob %s/%s created", job.Namespace, job.Name))

	if !apiequality.Semantic.DeepEqual(*oldStatus, job.Status) {
		if err := r.updateDIJobStatusInCluster(context.Background(), job); err != nil {
			log.Error(err, fmt.Sprintf("failed to update DIJob %s/%s status", job.Namespace, job.Name))
		}
	}
}

func (r *DIJobReconciler) markIncorrectJobFailed(obj client.Object) {
	log := r.Log.WithValues("dijob", diutil.NamespacedName(obj.GetNamespace(), obj.GetName()))

	// create dynamic client
	config := ctrl.GetConfigOrDie()
	dclient, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Error(err, "failed to create dynamic client")
		return
	}

	// dynamic client for dijobs
	dijobRes := schema.GroupVersionResource{
		Group:    div1alpha1.GroupVersion.Group,
		Version:  div1alpha1.GroupVersion.Version,
		Resource: "dijobs",
	}

	// build status
	failedConvertDIJob := fmt.Sprintf("failed to convert type %T to v1alpha1.DIJob", obj)
	status := div1alpha1.DIJobStatus{
		Phase: div1alpha1.JobFailed,
		Conditions: []div1alpha1.DIJobCondition{
			{
				Type:    div1alpha1.JobFailed,
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

func buildPodAndServiceForReplica(template *corev1.PodTemplateSpec, job *div1alpha1.DIJob,
	replicaType string, volumes []corev1.Volume) (*corev1.Pod, *corev1.Service, string, error) {
	if string(job.Spec.PriorityClassName) != "" {
		template.Spec.PriorityClassName = string(job.Spec.PriorityClassName)
	}

	// set restart policy for coordinator
	if replicaType == dicommon.CoordinatorName && template.Spec.RestartPolicy == "" {
		template.Spec.RestartPolicy = corev1.RestartPolicyNever
	}

	// build owner reference
	ownRefer := diutil.NewOwnerReference(job.APIVersion, job.Kind, job.Name, job.UID, true)

	// build pod
	pod, svc, port, err := diutil.BuildPodAndService(template, ownRefer, job.Name, job.Namespace, replicaType, volumes)
	if err != nil {
		return nil, nil, "", err
	}

	// access url
	url := diutil.ConcatURL(svc.Name, svc.Namespace, port)

	return pod, svc, url, nil
}

// func (r *DIJobReconciler) UpdateDIJob(ctx context.Context, job *div1alpha1.DIJob) error {
// 	var err error
// 	for i := 0; i < statusUpdateRetries; i++ {
// 		newJob := &div1alpha1.DIJob{}
// 		err = r.Get(ctx, types.NamespacedName{Namespace: job.Namespace, Name: job.Name}, newJob)
// 		if err != nil {
// 			break
// 		}

// 		err = r.Update(ctx, job, &client.UpdateOptions{})
// 		if err == nil {
// 			break
// 		}
// 	}
// 	return err
// }
