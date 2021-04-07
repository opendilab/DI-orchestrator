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

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=nervex.sensetime.com,resources=nervexjobs;actorlearnerconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nervex.sensetime.com,resources=nervexjobs/status;actorlearnerconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nervex.sensetime.com,resources=nervexjobs/finalizers;actorlearnerconfigs/finalizers,verbs=update
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
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	jobStatus := nvxJob.Status.DeepCopy()

	// list pods of NerveXJob
	pods, err := r.listPods(ctx, nvxJob)
	if err != nil {
		log.Error(err, "unable to list pods of NerveXJob")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// classify pods
	_, _, coordinator, err := nervexutil.ClassifyPods(pods)
	if err != nil {
		log.Error(err, "unable to classify pods")
	}

	// check the phase of NerveXJob
	// if isSucceeded(nvxJob) || isFailed(nvxJob) {
	// 	// delete actors and learners owned by nvcJob
	// 	// if err := r.deletePods(ctx, actors); err != nil {
	// 	// 	return ctrl.Result{}, client.IgnoreNotFound(err)
	// 	// }
	// 	// if err := r.deletePods(ctx, learners); err != nil {
	// 	// 	return ctrl.Result{}, client.IgnoreNotFound(err)
	// 	// }
	// }

	if err := r.reconcilePods(ctx, nvxJob, coordinator); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// update status
	if !apiequality.Semantic.DeepEqual(*jobStatus, nvxJob.Status) {
		if err := r.updateNerveXJobStatus(ctx, nvxJob); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}
	return ctrl.Result{}, nil
}

func (r *NerveXJobReconciler) listPods(ctx context.Context, job *nervexv1alpha1.NerveXJob) ([]*corev1.Pod, error) {
	podList := &corev1.PodList{}

	// generate label selector
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: nervexutil.GenLabels(job.Name),
	})
	if err != nil {
		return nil, err
	}

	// list pods of job
	err = r.List(ctx, podList, &client.ListOptions{Namespace: job.Namespace, LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}

	pods := []*corev1.Pod{}
	for _, pod := range podList.Items {
		pods = append(pods, pod.DeepCopy())
	}
	return pods, nil
}

// func (r *NerveXJobReconciler) deletePods(ctx context.Context, pods []*corev1.Pod) error {
// 	for _, pod := range pods {
// 		if err := r.Delete(ctx, pod, &client.DeleteOptions{}); err != nil {
// 			return client.IgnoreNotFound(err)
// 		}
// 	}
// 	return nil
// }

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
