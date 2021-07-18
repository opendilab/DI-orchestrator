package controllers

import (
	"context"
	"fmt"

	mapset "github.com/deckarep/golang-set"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div1alpha1 "opendilab.org/di-orchestrator/api/v1alpha1"
	dicommon "opendilab.org/di-orchestrator/common"
	diutil "opendilab.org/di-orchestrator/utils"
)

func (r *DIJobReconciler) reconcilePodsAndServices(ctx context.Context, job *div1alpha1.DIJob, pods []*corev1.Pod, services []*corev1.Service) error {
	pmap, smap := make(map[string]*corev1.Pod), make(map[string]*corev1.Service)
	pset, sset := mapset.NewSet(), mapset.NewSet()
	for _, pod := range pods {
		pset.Add(pod.Name)
		pmap[pod.Name] = pod
	}

	for _, service := range services {
		sset.Add(service.Name)
		smap[service.Name] = service
	}

	// pods that don't have services
	dpset := pset.Difference(sset)
	for dp := range dpset.Iter() {
		pod := pmap[dp.(string)]
		port, ok := diutil.GetDefaultPortFromPod(pod)
		if !ok {
			port = diutil.GetReplicaDefaultPort(pod.Labels[dicommon.ReplicaTypeLabel])
		}
		svc := diutil.BuildService(pod.Name, pod.Namespace, pod.OwnerReferences[0], pod.GetLabels(), port)
		svc.SetOwnerReferences(pod.GetOwnerReferences())

		// if pod is ddp learner, we need to add gpu ports to service
		if pod.Labels[dicommon.ReplicaTypeLabel] == dicommon.DDPLearnerName {
			rs := diutil.GetPodResources(pod)
			gpus := int(rs.GPU.Value())
			diutil.AddPortsToService(svc, gpus, port)
		}
		if err := r.createService(ctx, job, svc); err != nil && !errors.IsAlreadyExists(err) {
			return err
		}
	}

	// services that don't have pods
	dsset := sset.Difference(pset)
	for ds := range dsset.Iter() {
		svc := smap[ds.(string)]
		if err := r.deleteService(ctx, job, svc); err != nil {
			return err
		}
	}
	return nil
}

func (r *DIJobReconciler) createPodAndService(ctx context.Context, job *div1alpha1.DIJob, pod *corev1.Pod, svc *corev1.Service) error {
	log := r.Log.WithValues("dijob", diutil.NamespacedName(job.Namespace, job.Name))

	log.Info("create pod ", "pod name:", pod)
	if err := r.createPod(ctx, job, pod); err != nil {
		return err
	}

	log.Info("create service ", "service name:", svc)
	if err := r.createService(ctx, job, svc); err != nil {
		return err
	}
	return nil
}

func (r *DIJobReconciler) createPod(ctx context.Context, job *div1alpha1.DIJob, pod *corev1.Pod) error {
	if err := r.Create(ctx, pod, &client.CreateOptions{}); err != nil && !errors.IsAlreadyExists(err) {
		msg := fmt.Sprintf("Failed to create pod: %s error: %v", pod.Name, err)
		r.Recorder.Eventf(job, corev1.EventTypeWarning, FailedCreateReason, msg)
		return fmt.Errorf("failed to create pod: %v", err)
	}
	msg := fmt.Sprintf("Create pod: %s", pod.Name)
	r.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulCreateReason, msg)
	return nil
}

func (r *DIJobReconciler) createService(ctx context.Context, job *div1alpha1.DIJob, service *corev1.Service) error {
	if err := r.Create(ctx, service, &client.CreateOptions{}); err != nil && !errors.IsAlreadyExists(err) {
		msg := fmt.Sprintf("Failed to create service: %s error: %v", service.Name, err)
		r.Recorder.Eventf(job, corev1.EventTypeWarning, FailedCreateReason, msg)
		return fmt.Errorf("failed to create service: %v", err)
	}
	msg := fmt.Sprintf("Create service: %s", service.Name)
	r.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulCreateReason, msg)
	return nil
}

func (r *DIJobReconciler) deletePod(ctx context.Context, job *div1alpha1.DIJob, pod *corev1.Pod) error {
	if err := r.Delete(ctx, pod,
		&client.DeleteOptions{GracePeriodSeconds: func(a int64) *int64 { return &a }(0)}); err != nil && !errors.IsNotFound(err) {
		msg := fmt.Sprintf("Failed to delete pod: %s error: %v", pod.Name, err)
		r.Recorder.Eventf(job, corev1.EventTypeWarning, FailedDeleteReason, msg)
		return fmt.Errorf("failed to delete pod: %v", err)
	}
	msg := fmt.Sprintf("Delete pod: %s", pod.Name)
	r.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulDeleteReason, msg)
	return nil
}

func (r *DIJobReconciler) deleteService(ctx context.Context, job *div1alpha1.DIJob, service *corev1.Service) error {
	if err := r.Delete(ctx, service,
		&client.DeleteOptions{GracePeriodSeconds: func(a int64) *int64 { return &a }(0)}); err != nil && !errors.IsNotFound(err) {
		msg := fmt.Sprintf("Failed to delete service: %s error: %v", service.Name, err)
		r.Recorder.Eventf(job, corev1.EventTypeWarning, FailedDeleteReason, msg)
		return fmt.Errorf("failed to delete service: %v", err)
	}
	msg := fmt.Sprintf("Delete service: %s", service.Name)
	r.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulDeleteReason, msg)
	return nil
}
