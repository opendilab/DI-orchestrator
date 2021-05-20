/*
Copyright 2021 The SensePhoenix authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
	nervexutil "go-sensephoenix.sensetime.com/nervex-operator/utils"
)

// NerveXJobReconciler reconciles a NerveXJob object
type NerveXJobReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	AGConfig string
}

//+kubebuilder:rbac:groups=nervex.sensetime.com,resources=nervexjobs;aggregatorconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nervex.sensetime.com,resources=nervexjobs/status;aggregatorconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nervex.sensetime.com,resources=nervexjobs/finalizers;aggregatorconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods;services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NerveXJob object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
func (r *NerveXJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("nervexjob", req.NamespacedName)
	log.Info("reconcile nervexjob", "nervexjob", req.NamespacedName)

	// get NerveXJob object
	nvxJob := &nervexv1alpha1.NerveXJob{}
	err := r.Get(ctx, req.NamespacedName, nvxJob)
	if err != nil {
		log.Error(client.IgnoreNotFound(err), "failed to get NerveXJob", "job", req.NamespacedName)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	jobStatus := nvxJob.Status.DeepCopy()

	// list pods of NerveXJob
	pods, err := nervexutil.ListPods(ctx, r.Client, nvxJob)
	if err != nil {
		log.Error(client.IgnoreNotFound(err), "failed to list pods of NerveXJob", "job", req.NamespacedName)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// list services of NerveXJob
	services, err := nervexutil.ListServices(ctx, r.Client, nvxJob)
	if err != nil {
		log.Error(client.IgnoreNotFound(err), "failed to list services of NerveXJob", "job", req.NamespacedName)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// check the phase of NerveXJob
	if isSucceeded(nvxJob) || isFailed(nvxJob) {
		if err := r.deletePodsAndServices(ctx, nvxJob, pods, services); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		return ctrl.Result{}, nil
	}

	// initialize NerveXJob status
	initializeNerveXJobReplicaStatus(nvxJob)

	if err := r.reconcilePods(ctx, nvxJob, pods); err != nil {
		log.Error(err, "failed to reconcile pods", "job", req.NamespacedName)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// update status
	defer func() {
		if !apiequality.Semantic.DeepEqual(*jobStatus, nvxJob.Status) {
			if err := r.updateNerveXJobStatus(ctx, nvxJob); err != nil {
				log.Error(err, "failed to update NerveXJobStatus", "job", req.NamespacedName)
			}
		}
	}()
	return ctrl.Result{}, nil
}

func (r *NerveXJobReconciler) deletePodsAndServices(ctx context.Context, job *nervexv1alpha1.NerveXJob, pods []*corev1.Pod, services []*corev1.Service) error {
	log := r.Log.WithValues("nervexjob", fmt.Sprintf("%s/%s", job.Namespace, job.Name))
	if len(pods) == 0 {
		return nil
	}

	// delete services of NerveXJob
	for _, svc := range services {
		if err := r.Delete(ctx, svc, &client.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	if job.Spec.CleanPodPolicy != nervexv1alpha1.CleanPodPolicyALL &&
		job.Spec.CleanPodPolicy != nervexv1alpha1.CleanPodPolicyRunning {
		return nil
	}

	for _, pod := range pods {
		// Just delete running pod when the cleanPodPolicy is Running
		needsDelete := true
		if job.Spec.CleanPodPolicy == nervexv1alpha1.CleanPodPolicyRunning {
			if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
				continue
			}
			// pods that are in crashLoopBackoff status are in Running phase, these pods should not be deleted
			for _, ctStatus := range pod.Status.ContainerStatuses {
				if ctStatus.State.Terminated != nil || ctStatus.State.Waiting != nil &&
					ctStatus.State.Waiting.Reason == "CrashLoopBackOff" {
					needsDelete = false
					break
				}
			}
		}

		// if pod is already in terminating state, do not delete it
		if nervexutil.IsTerminating(pod) {
			needsDelete = false
		}
		if !needsDelete {
			continue
		}

		msg := fmt.Sprintf("Delete pod %s of job %s/%s, since the CleanPodPolicy is %s", pod.Name, job.Namespace, job.Name, job.Spec.CleanPodPolicy)
		log.Info(msg)
		if err := r.Delete(ctx, pod, &client.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NerveXJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nervexv1alpha1.NerveXJob{}).
		Watches(
			&source.Kind{Type: &corev1.Pod{}},
			&handler.EnqueueRequestForOwner{
				IsController: true,
				OwnerType:    &nervexv1alpha1.NerveXJob{},
			},
			builder.Predicates{},
		).
		Complete(r)
}