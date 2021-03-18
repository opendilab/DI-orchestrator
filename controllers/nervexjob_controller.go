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
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
)

// NervexJobReconciler reconciles a NervexJob object
type NervexJobReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=nervex.sensetime.com,resources=nervexjobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nervex.sensetime.com,resources=nervexjobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nervex.sensetime.com,resources=nervexjobs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NervexJob object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
func (r *NervexJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("nervexjob", req.NamespacedName)

	// get NervexJob object
	nvxJob := &nervexv1alpha1.NervexJob{}
	err := r.Get(ctx, req.NamespacedName, nvxJob)
	if err != nil {
		log.Error(err, "unable to fetch NervexJob")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// list pods of NervexJob
	pods, err := r.listPods(ctx, nvxJob)
	if err != nil {
		log.Error(err, "unable to list pods of NervexJob")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// classify pods
	actors, learners, coordinator, err := r.classifyPods(pods)
	if err != nil {
		log.Error(err, "unable to classify pods")
	}

	// check the phase of NervexJob
	if isSucceeded(nvxJob) || isFailed(nvxJob) {
		// delete actors and learners owned by nvcJob
		if err := r.deletePods(ctx, actors); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		if err := r.deletePods(ctx, learners); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	if err := r.reconcilePods(ctx, actors, learners, coordinator); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, nil
}

func (r *NervexJobReconciler) reconcilePods(ctx context.Context, actors []*corev1.Pod, learners []*corev1.Pod, coordinator *corev1.Pod) error {
	if coordinator != nil {

	}
	return nil
}

func (r *NervexJobReconciler) listPods(ctx context.Context, job *nervexv1alpha1.NervexJob) ([]*corev1.Pod, error) {
	podList := &corev1.PodList{}

	// generate label selector
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: GenLabels(job.Name),
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
		pods = append(pods, &pod)
	}
	return pods, nil
}

func (r *NervexJobReconciler) classifyPods(pods []*corev1.Pod) (actors []*corev1.Pod, learners []*corev1.Pod, coordinator *corev1.Pod, err error) {
	// filter out actors
	actors, err = filterOutReplicaPods(pods, ActorName)
	if err != nil {
		return
	}

	// filter out leader pods
	learners, err = filterOutReplicaPods(pods, LearnerName)
	if err != nil {
		return
	}

	// filter out coordinator pod
	coordinators := []*corev1.Pod{}
	coordinators, err = filterOutReplicaPods(pods, CoordinatorName)
	if err != nil {
		return
	}

	if len(coordinators) > 1 {
		err = fmt.Errorf("there must be only one coordinator")
		return
	}
	if len(coordinators) < 1 {
		return
	}
	coordinator = coordinators[0]
	return
}

func filterOutReplicaPods(pods []*corev1.Pod, replicaType string) ([]*corev1.Pod, error) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{ReplicaTypeLabel: replicaType},
	})
	if err != nil {
		return nil, err
	}

	result := []*corev1.Pod{}
	for _, pod := range pods {
		if !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		result = append(result, pod)
	}
	return result, nil
}

func (r *NervexJobReconciler) deletePods(ctx context.Context, pods []*corev1.Pod) error {
	for _, pod := range pods {
		if err := r.Delete(ctx, pod, &client.DeleteOptions{}); err != nil {
			return client.IgnoreNotFound(err)
		}
	}
	return nil
}

func GenLabels(jobName string) map[string]string {
	groupName := nervexv1alpha1.GroupVersion.Group
	return map[string]string{
		GroupNameLabel:      groupName,
		JobNameLabel:        strings.Replace(jobName, "/", "-", -1),
		ControllerNameLabel: ControllerName,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *NervexJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nervexv1alpha1.NervexJob{}).
		Complete(r)
}
