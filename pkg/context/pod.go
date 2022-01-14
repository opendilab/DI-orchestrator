package context

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
	pod.OwnerReferences = append(pod.OwnerReferences, diutil.NewOwnerReference(job.APIVersion, job.Kind, job.Name, job.UID, true))
	if nodeName != "" {
		// TODO(liqingping): add node selector to pod
	}
	pod.Spec.RestartPolicy = corev1.RestartPolicyNever
	pod.Spec.Hostname = pod.Name
	pod.Spec.Subdomain = job.Name

	labels := diutil.GenLabels(*job)
	labels[dicommon.LabelRank] = strconv.Itoa(rank)
	labels[dicommon.LabelPod] = pod.Name
	diutil.AddLabelsToPod(pod, labels)

	annotations := map[string]string{
		dicommon.AnnotationGeneration: strconv.Itoa(generation),
		dicommon.AnnotationReplicas:   strconv.Itoa(replicas),
		dicommon.AnnotationRank:       strconv.Itoa(rank),
		dicommon.AnnotationNode:       nodeName,
	}
	diutil.AddAnnotationsToPod(pod, annotations)

	envs := map[string]string{
		dicommon.ENVJobID:         diutil.NamespacedName(job.Namespace, job.Name),
		dicommon.ENVJobGeneration: strconv.Itoa(generation),
		dicommon.ENVServerURL:     dicommon.GetDIServerURL(),
	}
	diutil.AddEnvsToPod(pod, envs)

	OnTopologyHandler(job, rank, pod)
	return pod
}

func (c *Context) BuildService(job *div1alpha2.DIJob) *corev1.Service {
	labels := diutil.GenLabels(*job)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      job.Name,
			Namespace: job.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				diutil.NewOwnerReference(job.APIVersion, job.Kind, job.Name, job.UID, true),
			},
			Labels: labels,
		},
		Spec: corev1.ServiceSpec{
			Selector:  labels,
			ClusterIP: "None",
			Ports: []corev1.ServicePort{
				{
					Port: dicommon.DefaultPort,
					Name: dicommon.DefaultPortName,
				},
			},
		},
	}
}

func (c *Context) CreatePod(job *div1alpha2.DIJob, pod *corev1.Pod) error {
	log := c.Log.WithName("CreatePod").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	if err := c.Create(c.ctx, pod, &client.CreateOptions{}); err != nil && !errors.IsAlreadyExists(err) {
		msg := fmt.Sprintf("failed to create pod: %s error: %v", pod.Name, err)
		c.Recorder.Eventf(job, corev1.EventTypeWarning, FailedCreateReason, msg)
		return fmt.Errorf("failed to create pod: %v", err)
	}
	msg := fmt.Sprintf("create pod %s", pod.Name)
	log.Info(msg)
	// c.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulCreateReason, msg)
	return nil
}

func (c *Context) CreateService(job *div1alpha2.DIJob, service *corev1.Service) error {
	log := c.Log.WithName("CreateService").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	if err := c.Create(c.ctx, service, &client.CreateOptions{}); err != nil && !errors.IsAlreadyExists(err) {
		msg := fmt.Sprintf("failed to create service: %s error: %v", service.Name, err)
		c.Recorder.Eventf(job, corev1.EventTypeWarning, FailedCreateReason, msg)
		return fmt.Errorf("failed to create service: %v", err)
	}
	msg := fmt.Sprintf("create service %s", service.Name)
	log.Info(msg)
	// c.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulCreateReason, msg)
	return nil
}

func (c *Context) DeletePodsAndServices(job *div1alpha2.DIJob, pods []*corev1.Pod, services []*corev1.Service) error {
	log := c.Log.WithName("DeletePodsAndServices").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
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

		msg := fmt.Sprintf("delete pod %s", pod.Name)
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
	log := c.Log.WithName("DeletePod").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	if err := c.Delete(c.ctx, pod,
		&client.DeleteOptions{GracePeriodSeconds: func(a int64) *int64 { return &a }(0)}); err != nil && !errors.IsNotFound(err) {
		msg := fmt.Sprintf("failed to delete pod: %s error: %v", pod.Name, err)
		c.Recorder.Eventf(job, corev1.EventTypeWarning, FailedDeleteReason, msg)
		return fmt.Errorf("failed to delete pod: %v", err)
	}
	msg := fmt.Sprintf("delete pod %s", pod.Name)
	log.Info(msg)
	// c.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulDeleteReason, msg)
	return nil
}

func (c *Context) DeleteService(job *div1alpha2.DIJob, service *corev1.Service) error {
	log := c.Log.WithName("DeleteService").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	if err := c.Delete(c.ctx, service,
		&client.DeleteOptions{GracePeriodSeconds: func(a int64) *int64 { return &a }(0)}); err != nil && !errors.IsNotFound(err) {
		msg := fmt.Sprintf("failed to delete service: %s error: %v", service.Name, err)
		c.Recorder.Eventf(job, corev1.EventTypeWarning, FailedDeleteReason, msg)
		return fmt.Errorf("failed to delete service: %v", err)
	}
	msg := fmt.Sprintf("delete service %s", service.Name)
	log.Info(msg)
	// c.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulDeleteReason, msg)
	return nil
}

func (c *Context) ListJobPods(job *div1alpha2.DIJob) ([]*corev1.Pod, error) {
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: diutil.GenLabels(*job),
	})
	if err != nil {
		return nil, err
	}

	// list pods of job
	pods, err := c.ListPods(&client.ListOptions{Namespace: job.Namespace, LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}
	return pods, nil
}

func (c *Context) ListJobServices(job *div1alpha2.DIJob) ([]*corev1.Service, error) {
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: diutil.GenLabels(*job),
	})
	if err != nil {
		return nil, err
	}

	// list svcs of job
	svcs, err := c.ListServices(&client.ListOptions{Namespace: job.Namespace, LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}
	return svcs, nil
}

func (c *Context) ListPods(opts *client.ListOptions) ([]*corev1.Pod, error) {
	podList := &corev1.PodList{}
	err := c.List(c.ctx, podList, opts)
	if err != nil {
		return nil, err
	}

	pods := []*corev1.Pod{}
	for _, pod := range podList.Items {
		pods = append(pods, pod.DeepCopy())
	}
	return pods, nil
}

func (c *Context) ListServices(opts *client.ListOptions) ([]*corev1.Service, error) {
	svcList := &corev1.ServiceList{}
	err := c.List(c.ctx, svcList, opts)
	if err != nil {
		return nil, err
	}

	svcs := []*corev1.Service{}
	for _, pod := range svcList.Items {
		svcs = append(svcs, pod.DeepCopy())
	}
	return svcs, nil
}
