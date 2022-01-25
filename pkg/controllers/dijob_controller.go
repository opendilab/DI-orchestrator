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

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
	dihandler "opendilab.org/di-orchestrator/pkg/common/handler"
	dicontext "opendilab.org/di-orchestrator/pkg/context"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

// DIJobReconciler reconciles a DIJob object
type DIJobReconciler struct {
	Scheme *runtime.Scheme
	ctx    dicontext.Context
}

func NewDIJobReconciler(scheme *runtime.Scheme, ctx dicontext.Context) *DIJobReconciler {
	return &DIJobReconciler{
		Scheme: scheme,
		ctx:    ctx,
	}
}

//+kubebuilder:rbac:groups=diengine.opendilab.org,resources=dijobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=diengine.opendilab.org,resources=dijobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=diengine.opendilab.org,resources=dijobs/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods;services;events;nodes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list

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
	log := r.ctx.Log.WithName("Reconcile").WithValues("job", req.NamespacedName)

	// get DIJob object
	job := &div2alpha1.DIJob{}
	err := r.ctx.Get(ctx, req.NamespacedName, job)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "failed to get job")
		}
		return ctrl.Result{}, nil
	}

	pods, err := r.ctx.ListJobPods(job)
	if err != nil {
		log.Error(err, "failed to list pods")
		return ctrl.Result{}, nil
	}

	services, err := r.ctx.ListJobServices(job)
	if err != nil {
		log.Error(err, "failed to list services")
		return ctrl.Result{}, nil
	}

	// check job phase
	if diutil.IsSucceeded(job) || diutil.IsFailed(job) {
		if err := r.ctx.DeletePodsAndServices(job, pods, services); err != nil {
			log.Error(err, "failed to delete pods and services")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, nil
	}

	if err := r.reconcileReplicas(ctx, job, pods, services); err != nil {
		log.Error(err, "failed to reconcile pods")
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DIJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&div2alpha1.DIJob{}).
		Watches(
			&source.Kind{Type: &div2alpha1.DIJob{}},
			&dihandler.EventHandler{
				OnCreateHandlers: []func(obj client.Object){
					r.onJobAddHandler,
				},
				OnUpdateHandlers: []func(old, new client.Object){
					r.onJobUpdateHandler,
				},
				OnDeleteHandlers: []func(obj client.Object){
					r.onJobDeleteHandler,
				},
			},
			builder.Predicates{},
		).
		Watches(
			&source.Kind{Type: &corev1.Pod{}},
			&handler.EnqueueRequestForOwner{
				IsController: true,
				OwnerType:    &div2alpha1.DIJob{},
			},
			builder.Predicates{},
		).
		Watches(
			&source.Kind{Type: &corev1.Service{}},
			&handler.EnqueueRequestForOwner{
				IsController: true,
				OwnerType:    &div2alpha1.DIJob{},
			},
		).
		Complete(r)
}

// addDIJob is the event handler responsible for handling job add events
func (r *DIJobReconciler) onJobAddHandler(obj client.Object) {
	jobkey := diutil.NamespacedName(obj.GetNamespace(), obj.GetName())
	log := r.ctx.Log.WithName("onJobAddHandler").WithValues("job", jobkey)
	job, ok := obj.(*div2alpha1.DIJob)
	if !ok {
		log.Error(fmt.Errorf("failed to convert object to DIJob"), "")
		r.ctx.MarkIncorrectJobFailed(obj)
		return
	}
	oldStatus := job.Status.DeepCopy()

	// update job status
	msg := "job created."
	if job.Status.Phase == "" {
		r.ctx.UpdateJobStatus(job, div2alpha1.JobPending, dicontext.DIJobPendingReason, msg)
		r.ctx.Recorder.Eventf(job, corev1.EventTypeNormal, dicontext.DIJobPendingReason, msg)
	}

	if !apiequality.Semantic.DeepEqual(*oldStatus, job.Status) {
		if err := r.ctx.UpdateDIJobStatusInCluster(job); err != nil {
			log.Error(err, "failed to update job status")
		}
	}
}

func (r *DIJobReconciler) onJobUpdateHandler(old, new client.Object) {
	jobkey := diutil.NamespacedName(old.GetNamespace(), old.GetName())
	log := r.ctx.Log.WithName("onJobUpdateHandler").WithValues("job", jobkey)
	oldjob, ok := old.(*div2alpha1.DIJob)
	if !ok {
		log.Error(fmt.Errorf("failed to convert object to DIJob"), "")
		return
	}
	newjob, ok := new.(*div2alpha1.DIJob)
	if !ok {
		log.Error(fmt.Errorf("failed to convert object to DIJob"), "")
		return
	}
	staleStatus := newjob.Status.DeepCopy()

	HandleJobStatus(r.ctx, oldjob, newjob)
	if !apiequality.Semantic.DeepEqual(*staleStatus, newjob.Status) {
		if err := r.ctx.UpdateDIJobStatusInCluster(newjob); err != nil {
			log.Error(err, "failed to update job status")
		}
	}
}

func (r *DIJobReconciler) onJobDeleteHandler(obj client.Object) {
	jobkey := diutil.NamespacedName(obj.GetNamespace(), obj.GetName())
	log := r.ctx.Log.WithName("onJobDeleteHandler").WithValues("job", jobkey)
	log.Info("job deleted.")
}
