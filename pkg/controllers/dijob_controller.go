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
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

// DIJobReconciler reconciles a DIJob object
type DIJobReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=diengine.opendilab.org,resources=dijobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=diengine.opendilab.org,resources=dijobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=diengine.opendilab.org,resources=dijobs/finalizers,verbs=update
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
	job := &div1alpha2.DIJob{}
	err := r.Get(ctx, req.NamespacedName, job)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "failed to get DIJob", "job", req.NamespacedName)
		}
		return ctrl.Result{}, nil
	}

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
		return ctrl.Result{}, nil
	}

	if err := r.reconcileReplicas(ctx, job, pods, services); err != nil {
		log.Error(err, "failed to reconcile pods", "job", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *DIJobReconciler) deletePodsAndServices(ctx context.Context, job *div1alpha2.DIJob, pods []*corev1.Pod, services []*corev1.Service) error {
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

	if job.Spec.CleanPodPolicy != div1alpha2.CleanPodPolicyAll &&
		job.Spec.CleanPodPolicy != div1alpha2.CleanPodPolicyRunning {
		return nil
	}

	for _, pod := range pods {
		// Just delete running pod when the cleanPodPolicy is Running
		needsDelete := true
		if job.Spec.CleanPodPolicy == div1alpha2.CleanPodPolicyRunning {
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
		For(&div1alpha2.DIJob{}).
		Watches(
			&source.Kind{Type: &div1alpha2.DIJob{}},
			&DIJobEventHandler{
				r,
			},
			builder.Predicates{},
		).
		Watches(
			&source.Kind{Type: &corev1.Pod{}},
			&handler.EnqueueRequestForOwner{
				IsController: true,
				OwnerType:    &div1alpha2.DIJob{},
			},
			builder.Predicates{},
		).
		Watches(
			&source.Kind{Type: &corev1.Service{}},
			&handler.EnqueueRequestForOwner{
				IsController: true,
				OwnerType:    &div1alpha2.DIJob{},
			},
		).
		Complete(r)
}

type DIJobEventHandler struct {
	r *DIJobReconciler
}

// Create implements EventHandler
func (e *DIJobEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	e.r.addDIJob(evt.Object)
}

// Update implements EventHandler
func (e *DIJobEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
}

// Delete implements EventHandler
func (e *DIJobEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
}

// Generic implements EventHandler
func (e *DIJobEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

// addDIJob is the event handler responsible for handling job add events
func (r *DIJobReconciler) addDIJob(obj client.Object) {
	log := r.Log.WithValues("dijob", diutil.NamespacedName(obj.GetNamespace(), obj.GetName()))
	job, ok := obj.(*div1alpha2.DIJob)
	if !ok {
		log.Error(fmt.Errorf("failed to convert object DIJob: %s/%s", obj.GetNamespace(), obj.GetName()), "")
		r.markIncorrectJobFailed(obj)
		return
	}
	oldStatus := job.Status.DeepCopy()

	// update job status
	msg := fmt.Sprintf("DIJob %s created", job.Name)
	r.updateJobStatus(context.Background(), job, div1alpha2.JobPending, DIJobPendingReason, msg)

	log.Info(fmt.Sprintf("DIJob %s/%s created", job.Namespace, job.Name))
	if !apiequality.Semantic.DeepEqual(*oldStatus, job.Status) {
		if err := r.updateDIJobStatusInCluster(context.Background(), job); err != nil {
			log.Error(err, fmt.Sprintf("failed to update DIJob %s/%s status", job.Namespace, job.Name))
		}
	}
}
