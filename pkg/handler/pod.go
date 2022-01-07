package handler

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

func (c *Context) BuildPod(job *div1alpha2.DIJob, rank int, allocation []string) *corev1.Pod {
	group := job.Spec.Group
	generation := int(job.Status.Generation)
	replicas := int(job.Status.Replicas)
	nodeName := ""
	if allocation != nil {
		nodeName = allocation[rank]
	}

	podTemplate := job.Spec.Template.DeepCopy()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: job.Namespace,
			Name:      diutil.ReplicaName(job.Name, generation, rank),
		},
		Spec: podTemplate.Spec,
	}

	labels := map[string]string{
		dicommon.LabelOperator: dicommon.OperatorName,
		dicommon.LabelJob:      job.Name,
		dicommon.LabelGroup:    group,
		dicommon.LabelRank:     strconv.Itoa(rank),
		dicommon.LabelPod:      pod.Name,
	}
	diutil.AddLabelsToPod(pod, labels)

	annotations := map[string]string{
		dicommon.AnnotationGeneration: strconv.Itoa(generation),
		dicommon.AnnotationReplicas:   strconv.Itoa(replicas),
		dicommon.AnnotationRank:       strconv.Itoa(rank),
		dicommon.AnnotationNode:       nodeName,
	}
	diutil.AddAnnotationsToPod(pod, annotations)

	pod.OwnerReferences = append(pod.OwnerReferences, diutil.NewOwnerReference(job.APIVersion, job.Kind, job.Name, job.UID, true))
	if nodeName != "" {
		// TODO(liqingping): add node selector to pod
	}
	pod.Spec.RestartPolicy = corev1.RestartPolicyNever

	envs := map[string]string{
		dicommon.ENVJobID:         diutil.NamespacedName(job.Namespace, job.Name),
		dicommon.ENVJobGeneration: strconv.Itoa(generation),
		dicommon.ENVServerURL:     dicommon.GetDIServerURL(),
	}
	diutil.AddEnvsToPod(pod, envs)

	OnTopologyHandler(job, rank, pod)
	return pod
}

func (c *Context) CreatePod(job *div1alpha2.DIJob, pod *corev1.Pod) error {
	if err := c.Create(c.ctx, pod, &client.CreateOptions{}); err != nil && !errors.IsAlreadyExists(err) {
		msg := fmt.Sprintf("Failed to create pod: %s error: %v", pod.Name, err)
		c.Recorder.Eventf(job, corev1.EventTypeWarning, FailedCreateReason, msg)
		return fmt.Errorf("failed to create pod: %v", err)
	}
	msg := fmt.Sprintf("Create pod: %s", pod.Name)
	c.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulCreateReason, msg)
	return nil
}

func (c *Context) CreateService(job *div1alpha2.DIJob, service *corev1.Service) error {
	if err := c.Create(c.ctx, service, &client.CreateOptions{}); err != nil && !errors.IsAlreadyExists(err) {
		msg := fmt.Sprintf("Failed to create service: %s error: %v", service.Name, err)
		c.Recorder.Eventf(job, corev1.EventTypeWarning, FailedCreateReason, msg)
		return fmt.Errorf("failed to create service: %v", err)
	}
	msg := fmt.Sprintf("Create service: %s", service.Name)
	c.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulCreateReason, msg)
	return nil
}

func (c *Context) DeletePodsAndServices(job *div1alpha2.DIJob, pods []*corev1.Pod, services []*corev1.Service) error {
	log := c.Log.WithValues("dijob", fmt.Sprintf("%s/%s", job.Namespace, job.Name))
	if len(pods) == 0 {
		return nil
	}
	for _, svc := range services {
		if err := c.DeleteService(job, svc); err != nil {
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
		if err := c.DeletePod(job, pod); err != nil {
			return err
		}
	}
	return nil
}

func (c *Context) DeletePods(job *div1alpha2.DIJob, pods []*corev1.Pod) error {
	var err error
	for _, pod := range pods {
		if err1 := c.DeletePod(job, pod); err != nil {
			err = err1
		}
	}
	return err
}

func (c *Context) DeletePod(job *div1alpha2.DIJob, pod *corev1.Pod) error {
	if err := c.Delete(c.ctx, pod,
		&client.DeleteOptions{GracePeriodSeconds: func(a int64) *int64 { return &a }(0)}); err != nil && !errors.IsNotFound(err) {
		msg := fmt.Sprintf("Failed to delete pod: %s error: %v", pod.Name, err)
		c.Recorder.Eventf(job, corev1.EventTypeWarning, FailedDeleteReason, msg)
		return fmt.Errorf("failed to delete pod: %v", err)
	}
	msg := fmt.Sprintf("Delete pod: %s", pod.Name)
	c.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulDeleteReason, msg)
	return nil
}

func (c *Context) DeleteService(job *div1alpha2.DIJob, service *corev1.Service) error {
	if err := c.Delete(c.ctx, service,
		&client.DeleteOptions{GracePeriodSeconds: func(a int64) *int64 { return &a }(0)}); err != nil && !errors.IsNotFound(err) {
		msg := fmt.Sprintf("Failed to delete service: %s error: %v", service.Name, err)
		c.Recorder.Eventf(job, corev1.EventTypeWarning, FailedDeleteReason, msg)
		return fmt.Errorf("failed to delete service: %v", err)
	}
	msg := fmt.Sprintf("Delete service: %s", service.Name)
	c.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulDeleteReason, msg)
	return nil
}

func (c *Context) ListPods(job *div1alpha2.DIJob) ([]*corev1.Pod, error) {
	podList := &corev1.PodList{}
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: diutil.GenLabels(*job),
	})
	if err != nil {
		return nil, err
	}

	// list pods of job
	err = c.List(c.ctx, podList, &client.ListOptions{Namespace: job.Namespace, LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}

	pods := []*corev1.Pod{}
	for _, pod := range podList.Items {
		pods = append(pods, pod.DeepCopy())
	}
	return pods, nil
}

func (c *Context) ListServices(job *div1alpha2.DIJob) ([]*corev1.Service, error) {
	svcList := &corev1.ServiceList{}
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: diutil.GenLabels(*job),
	})
	if err != nil {
		return nil, err
	}

	// list svcs of job
	err = c.List(c.ctx, svcList, &client.ListOptions{Namespace: job.Namespace, LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}

	svcs := []*corev1.Service{}
	for _, svc := range svcList.Items {
		svcs = append(svcs, svc.DeepCopy())
	}
	return svcs, nil
}
