package controllers

import (
	"context"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

func isSucceeded(job *div1alpha2.DIJob) bool {
	return job.Status.Phase == div1alpha2.JobSucceeded
}

func isFailed(job *div1alpha2.DIJob) bool {
	return job.Status.Phase == div1alpha2.JobFailed
}

func (r *DIJobReconciler) reconcileReplicas(ctx context.Context, job *div1alpha2.DIJob,
	pods []*corev1.Pod, services []*corev1.Service) error {

	log := r.Log.WithValues("dijob", diutil.NamespacedName(job.Namespace, job.Name))
	oldStatus := job.Status.DeepCopy()
	allocation := job.Status.Allocation
	replicas := int(job.Status.Replicas)
	if r.checkJobCompletion(ctx, job, pods) {
		log.Info("job completed:", job.Name, job.Status.Phase)
		return nil
	}

	switch job.Status.Phase {
	case div1alpha2.JobPending:
		if job.Spec.Preemptible {
			if allocation != nil && len(pods) == 0 {
				r.updateJobStatus(ctx, job, div1alpha2.JobStarting, DIJobStartingReason, fmt.Sprintf("DIJob %s is starting now.", job.Name))
			}
		} else {
			if job.Status.Replicas != 0 && len(pods) == 0 {
				r.updateJobStatus(ctx, job, div1alpha2.JobStarting, DIJobStartingReason, fmt.Sprintf("DIJob %s is starting now.", job.Name))
			}
		}
	case div1alpha2.JobStarting:
		if job.Spec.Preemptible {
			if diutil.CountPodsScheduled(pods) != replicas && r.detectRestart(job, pods, allocation, replicas) || allocation == nil {
				r.updateJobStatus(ctx, job, div1alpha2.JobRestarting, DIJobRestartingReason,
					fmt.Sprintf("DIJob %s is restarting since conditions changed.", job.Name))
			}

			if allocation != nil && len(pods) == 0 {
				job.Status.Generation = job.Status.Generation + 1
				createdPods := []*corev1.Pod{}
				for i := 0; i < len(allocation); i++ {
					pod, err := r.buildAndCreatePod(ctx, job, i, allocation)
					if err != nil {
						log.Error(err, "failed to create pod", "pod", pod.Name)
						r.updateJobStatus(ctx, job, div1alpha2.JobFailed, DIJobFailedReason,
							fmt.Sprintf("DIJob %s failed because pod %s failed to created.", job.Name, pod.Name))
						return r.deletePods(ctx, job, createdPods)
					}
					createdPods = append(createdPods, pod)
				}
			}
		} else {
			if replicas != 0 && len(pods) == 0 {
				createdPods := []*corev1.Pod{}
				for i := 0; i < replicas; i++ {
					pod, err := r.buildAndCreatePod(ctx, job, i, allocation)
					if err != nil {
						log.Error(err, "failed to create pod", "pod", pod.Name)
						r.updateJobStatus(ctx, job, div1alpha2.JobFailed, DIJobFailedReason,
							fmt.Sprintf("DIJob %s failed because pod %s failed to created.", job.Name, pod.Name))
						return r.deletePods(ctx, job, createdPods)
					}
					createdPods = append(createdPods, pod)
				}
			}
		}

		if len(pods) != replicas {
			r.updateJobStatus(ctx, job, div1alpha2.JobRestarting, DIJobRestartingReason,
				fmt.Sprintf("DIJob %s is restarting since the created pods %s are not matched replicas %s.", job.Name, len(pods), replicas))
		}

		if diutil.CountReadyPods(pods) == replicas {
			r.updateJobStatus(ctx, job, div1alpha2.JobRunning, DIJobRunningReason,
				fmt.Sprintf("DIJob %s is running since all pods are ready.", job.Name))
		}
	case div1alpha2.JobRunning:
		if r.detectRestart(job, pods, allocation, replicas) || len(pods) == 0 {
			r.updateJobStatus(ctx, job, div1alpha2.JobRestarting, DIJobRestartingReason,
				fmt.Sprintf("DIJob %s is restarting since the created pods %s are not matched replicas %s.", job.Name, len(pods), replicas))
		}
	case div1alpha2.JobRestarting:
		if len(pods) != 0 {
			r.deletePods(ctx, job, pods)
		} else {
			r.updateJobStatus(ctx, job, div1alpha2.JobPending, DIJobPendingReason,
				fmt.Sprintf("DIJob %s is pending since job restarted.", job.Name))
		}
	}

	if allocation != nil {
		job.Status.Replicas = int32(len(allocation))
		job.Status.ReadyReplicas = int32(diutil.CountReadyPods(pods))
	} else if job.Spec.Preemptible {
		job.Status.Allocation = nil
		job.Status.Replicas = 0
		job.Status.ReadyReplicas = 0
	}

	if !apiequality.Semantic.DeepEqual(*oldStatus, job.Status) {
		if err := r.updateDIJobStatusInCluster(ctx, job); err != nil {
			log.Error(err, "failed to update DIJobStatus", "job", job.Name)
			return err
		}
	}
	return nil
}

func (r *DIJobReconciler) detectRestart(job *div1alpha2.DIJob, pods []*corev1.Pod, allocation []string, replicas int) bool {
	log := r.Log.WithValues("detect restart")
	for _, pod := range pods {
		areplicas, err := strconv.Atoi(pod.Annotations[dicommon.AnnotationReplicas])
		if err != nil {
			log.Error(err, fmt.Sprintf("%s is not a valid number, mark job as restarted.", pod.Annotations[dicommon.AnnotationReplicas]))
			return true
		}
		rank, err := strconv.Atoi(pod.Annotations[dicommon.AnnotationRank])
		if err != nil {
			log.Error(err, fmt.Sprintf("%s is not a valid number, mark job as restarted.", pod.Annotations[dicommon.AnnotationRank]))
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
		Group:    div1alpha2.GroupVersion.Group,
		Version:  div1alpha2.GroupVersion.Version,
		Resource: "dijobs",
	}

	// build status
	failedConvertDIJob := fmt.Sprintf("failed to convert type %T to v1alpha2.DIJob", obj)
	status := div1alpha2.DIJobStatus{
		Phase: div1alpha2.JobFailed,
		Conditions: []div1alpha2.DIJobCondition{
			{
				Type:    div1alpha2.JobFailed,
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
