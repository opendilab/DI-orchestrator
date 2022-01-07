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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
	dihandler "opendilab.org/di-orchestrator/pkg/handler"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

// DIJobReconciler reconciles a DIJob object
type DIJobReconciler struct {
	Scheme *runtime.Scheme
	ctx    *dihandler.Context
}

func NewDIJobReconciler(scheme *runtime.Scheme, ctx *dihandler.Context) *DIJobReconciler {
	return &DIJobReconciler{
		Scheme: scheme,
		ctx:    ctx,
	}
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
	log := r.ctx.Log.WithValues("dijob", req.NamespacedName)

	// get DIJob object
	job := &div1alpha2.DIJob{}
	err := r.ctx.Get(ctx, req.NamespacedName, job)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "failed to get DIJob", "job", req.NamespacedName)
		}
		return ctrl.Result{}, nil
	}

	pods, err := r.ctx.ListPods(job)
	if err != nil {
		log.Error(err, "failed to list pods of DIJob", "job", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	services, err := r.ctx.ListServices(job)
	if err != nil {
		log.Error(err, "failed to list services of DIJob", "job", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	// check the phase of DIJob
	if diutil.IsSucceeded(job) || diutil.IsFailed(job) {
		if err := r.ctx.DeletePodsAndServices(job, pods, services); err != nil {
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

// SetupWithManager sets up the controller with the Manager.
func (r *DIJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&div1alpha2.DIJob{}).
		Watches(
			&source.Kind{Type: &div1alpha2.DIJob{}},
			&dihandler.DIJobEventHandler{
				Context: r.ctx,
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
