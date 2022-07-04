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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
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
//+kubebuilder:rbac:groups="",resources=pods;services;events,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=namespaces;nodes,verbs=get;list;watch

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
			log.Error(err, "get job.")
		}
		return ctrl.Result{}, nil
	}

	// validate job
	validators := make(diutil.Validators, 0)
	validators = append(validators, diutil.TaskTypeNameValidator)
	// check the task without name and set default name with task.Type

	//find task without name
	r.ctx.SetDefaultJobNameInCluster(ctx, job)

	if err := validators.Apply(job); err != nil {
		log.Error(err, "job validation.")
		old := job.DeepCopy()
		r.ctx.UpdateJobStatus(job, div2alpha1.JobFailed, dicontext.DIJobFailedReason, err.Error())
		if err := r.ctx.UpdateJobPhaseAndConditionsInCluster(ctx, old, job); err != nil {
			log.Error(err, "update job phase and conditions.")
		}
		return ctrl.Result{}, nil
	}

	pods, err := r.ctx.ListJobPods(ctx, job)
	if err != nil {
		log.Error(err, "list pods.")
		return ctrl.Result{}, nil
	}

	services, err := r.ctx.ListJobServices(ctx, job)
	if err != nil {
		log.Error(err, "list services.")
		return ctrl.Result{}, nil
	}

	// check job phase
	if diutil.IsSucceeded(job) || diutil.IsFailed(job) {
		if err := r.ctx.DeletePodsAndServices(ctx, job, pods, services); err != nil {
			log.Error(err, "delete pods and services.")
			return ctrl.Result{}, nil
		}

		old := job.DeepCopy()
		job.Status.ReadyReplicas = 0
		job.Status.TaskStatus = nil
		if err := r.ctx.UpdateJobReplicaStatusInCluster(ctx, old, job); err != nil {
			log.Error(err, "update job replica status.")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, nil
	}

	if err := r.reconcileReplicas(ctx, job, pods, services); err != nil {
		log.Error(err, "reconcile pods.")
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
			&dicommon.EventHandler{
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
		log.Error(fmt.Errorf("convert object to dijob"), "")
		r.ctx.MarkIncorrectJobFailed(context.Background(), obj)
		return
	}
	old := job.DeepCopy()

	// update job status
	msg := "job created."
	if job.Status.Phase == "" || job.Status.Phase == div2alpha1.JobPending {
		r.ctx.UpdateJobStatus(job, div2alpha1.JobPending, dicontext.DIJobPendingReason, msg)
		r.ctx.Recorder.Eventf(job, corev1.EventTypeNormal, dicontext.DIJobPendingReason, msg)
	}

	if err := r.ctx.UpdateJobPhaseAndConditionsInCluster(context.Background(), old, job); err != nil {
		log.Error(err, "update job phase and conditions.")
	}
}

func (r *DIJobReconciler) onJobUpdateHandler(old, new client.Object) {
	jobkey := diutil.NamespacedName(old.GetNamespace(), old.GetName())
	log := r.ctx.Log.WithName("onJobUpdateHandler").WithValues("job", jobkey)
	oldjob, ok := old.(*div2alpha1.DIJob)
	if !ok {
		log.Error(fmt.Errorf("convert object to dijob"), "")
		log.Error(fmt.Errorf("onvert object to dijob"), "")
		return
	}
	newjob, ok := new.(*div2alpha1.DIJob)
	if !ok {
		log.Error(fmt.Errorf("convert object to dijob"), "")
		return
	}
	stale := newjob.DeepCopy()

	HandleJobStatus(r.ctx, oldjob, newjob)
	if err := r.ctx.UpdateJobRestartsAndReschedulesInCluster(context.Background(), stale, newjob); err != nil {
		log.Error(err, "update job restarts and reschedules.")
	}
	if err := r.ctx.UpdateJobReplicaStatusInCluster(context.Background(), stale, newjob); err != nil {
		log.Error(err, "update job replica status.")
	}
}

func (r *DIJobReconciler) onJobDeleteHandler(obj client.Object) {
	jobkey := diutil.NamespacedName(obj.GetNamespace(), obj.GetName())
	log := r.ctx.Log.WithName("onJobDeleteHandler").WithValues("job", jobkey)
	log.Info("job deleted.")
}
