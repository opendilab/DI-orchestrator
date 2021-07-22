/*
Copyright 2021 The OpenDILab authors.

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
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	div1alpha1 "opendilab.org/di-orchestrator/api/v1alpha1"
	diutil "opendilab.org/di-orchestrator/utils"
)

// DIJobReconciler reconciles a DIJob object
type DIJobReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	AGConfig string
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=diengine.opendilab.org,resources=dijobs;aggregatorconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=diengine.opendilab.org,resources=dijobs/status;aggregatorconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=diengine.opendilab.org,resources=dijobs/finalizers;aggregatorconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods;services;events,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=namespaces;nodes,verbs=get;list

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DIJob object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
func (r *DIJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("dijob", req.NamespacedName)
	// log.Info("reconcile dijob", "dijob", req.NamespacedName)

	// get DIJob object
	job := &div1alpha1.DIJob{}
	err := r.Get(ctx, req.NamespacedName, job)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "failed to get DIJob", "job", req.NamespacedName)
		}
		return ctrl.Result{}, nil
	}

	jobStatus := job.Status.DeepCopy()

	// update status
	defer func() {
		if !apiequality.Semantic.DeepEqual(*jobStatus, job.Status) {
			if err := r.updateDIJobStatusInCluster(ctx, job); err != nil {
				log.Error(err, "failed to update DIJobStatus", "job", req.NamespacedName)
			}
		}
	}()

	// list pods of DIJob
	pods, err := diutil.ListPods(ctx, r.Client, job)
	if err != nil {
		log.Error(err, "failed to list pods of DIJob", "job", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	// list services of DIJob
	services, err := diutil.ListServices(ctx, r.Client, job)
	if err != nil {
		log.Error(err, "failed to list services of DIJob", "job", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	// check the phase of DIJob
	if isSucceeded(job) || isFailed(job) {
		if err := r.deletePodsAndServices(ctx, job, pods, services); err != nil {
			log.Error(err, "failed to delete pods and services of DIJob", "job", req.NamespacedName)
			return ctrl.Result{}, nil
		}

		if isSucceeded(job) {
			for rtype := range job.Status.ReplicaStatus {
				job.Status.ReplicaStatus[rtype].Succeeded += job.Status.ReplicaStatus[rtype].Active
				job.Status.ReplicaStatus[rtype].Active = 0
			}
		}
		return ctrl.Result{}, nil
	}

	// initialize DIJob status
	initializeDIJobReplicaStatus(job)

	if err := r.reconcileReplicas(ctx, job, pods, services); err != nil {
		log.Error(err, "failed to reconcile pods", "job", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *DIJobReconciler) deletePodsAndServices(ctx context.Context, job *div1alpha1.DIJob, pods []*corev1.Pod, services []*corev1.Service) error {
	log := r.Log.WithValues("dijob", fmt.Sprintf("%s/%s", job.Namespace, job.Name))
	if len(pods) == 0 {
		return nil
	}

	// delete services of DIJob
	for _, svc := range services {
		if err := r.deleteService(ctx, job, svc); err != nil {
			return err
		}
	}

	if job.Spec.CleanPodPolicy != div1alpha1.CleanPodPolicyALL &&
		job.Spec.CleanPodPolicy != div1alpha1.CleanPodPolicyRunning {
		return nil
	}

	for _, pod := range pods {
		// Just delete running pod when the cleanPodPolicy is Running
		needsDelete := true
		if job.Spec.CleanPodPolicy == div1alpha1.CleanPodPolicyRunning {
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
		if diutil.IsTerminating(pod) {
			needsDelete = false
		}
		if !needsDelete {
			continue
		}

		msg := fmt.Sprintf("Delete pod %s of job %s/%s", pod.Name, job.Namespace, job.Name)
		log.Info(msg)
		if err := r.deletePod(ctx, job, pod); err != nil {
			return err
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DIJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&div1alpha1.DIJob{}).
		Watches(
			&source.Kind{Type: &corev1.Pod{}},
			&handler.EnqueueRequestForOwner{
				IsController: true,
				OwnerType:    &div1alpha1.DIJob{},
			},
			builder.Predicates{},
		).
		Watches(
			&source.Kind{Type: &corev1.Service{}},
			&handler.EnqueueRequestForOwner{
				IsController: true,
				OwnerType:    &div1alpha1.DIJob{},
			},
		).
		Complete(r)
}
