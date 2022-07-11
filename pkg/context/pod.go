package context

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

func (c *Context) BuildPod(job *div2alpha1.DIJob, rank int, allocation []string, taskNum int, taskLocalRank int) *corev1.Pod {
	replicas := 0
	for _, task := range job.Spec.Tasks {
		replicas += int(task.Replicas)
	}

	podTemplate := job.Spec.Tasks[taskNum].Template.DeepCopy()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: job.Namespace,
			Name:      diutil.ReplicaName(job.Name, job.Spec.Tasks[taskNum].Name, taskLocalRank),
		},
		Spec: podTemplate.Spec,
	}
	pod.OwnerReferences = append(pod.OwnerReferences, *metav1.NewControllerRef(job, div2alpha1.GroupVersion.WithKind(div2alpha1.KindDIJob)))

	nodeName := ""
	if job.Spec.Preemptible {
		if len(allocation) >= rank {
			nodeName = allocation[rank]
			pod.Spec.NodeName = nodeName
		}
	}

	pod.Spec.RestartPolicy = corev1.RestartPolicyNever
	// set hostname and subdomain to enable service subdomain
	pod.Spec.Hostname = pod.Name
	pod.Spec.Subdomain = svcName(job.Name)

	labels := diutil.GenLabels(*job)
	labels[dicommon.LabelPod] = pod.Name
	labels[dicommon.LabelTaskType] = string(job.Spec.Tasks[taskNum].Type)
	labels[dicommon.LabelTaskName] = job.Spec.Tasks[taskNum].Name
	diutil.AddLabelsToPod(pod, labels)

	annotations := map[string]string{
		dicommon.AnnotationReplicas: strconv.Itoa(replicas),
		dicommon.AnnotationRank:     strconv.Itoa(rank),
		dicommon.AnnotationNode:     nodeName,
		dicommon.AnnotationTaskRank: strconv.Itoa(taskLocalRank),
	}
	diutil.AddAnnotationsToPod(pod, annotations)

	// build fqdns for all pods
	var fqdns []string
	var internalFqdns []string

	for num, task := range job.Spec.Tasks {
		for i := 0; i < int(task.Replicas); i++ {
			replicaName := diutil.ReplicaName(job.Name, job.Spec.Tasks[num].Name, i)
			fqdn := diutil.PodFQDN(replicaName, pod.Spec.Subdomain, job.Namespace, dicommon.GetServiceDomainName())
			fqdns = append(fqdns, fqdn)
			if task.Type == job.Spec.Tasks[taskNum].Type {
				internalFqdns = append(internalFqdns, fqdn)
			}
		}
	}
	DITaskNodes := fmt.Sprintf(dicommon.ENVTaskNodesFormat, strings.ReplaceAll(strings.ToUpper(job.Spec.Tasks[taskNum].Name), "-", "_"))
	envs := map[string]string{
		dicommon.ENVJobID:     diutil.NamespacedName(job.Namespace, job.Name),
		dicommon.ENVServerURL: dicommon.GetDIServerURL(),
		dicommon.ENVRank:      strconv.Itoa(rank),
		dicommon.ENVNodes:     strings.Join(fqdns, ","),
		DITaskNodes:           strings.Join(internalFqdns, ","),
	}
	diutil.AddEnvsToPod(pod, envs)

	// add job volumes to pod
	if pod.Spec.Volumes == nil {
		pod.Spec.Volumes = make([]corev1.Volume, 0)
	}
	pod.Spec.Volumes = append(pod.Spec.Volumes, job.Spec.Volumes...)

	return pod
}

func (c *Context) BuildService(job *div2alpha1.DIJob) *corev1.Service {
	labels := diutil.GenLabels(*job)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName(job.Name),
			Namespace: job.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(job, div2alpha1.GroupVersion.WithKind(div2alpha1.KindDIJob)),
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

func (c *Context) CreatePod(ctx context.Context, job *div2alpha1.DIJob, pod *corev1.Pod) error {
	log := c.Log.WithName("CreatePod").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	if err := c.Create(ctx, pod, &client.CreateOptions{}); err != nil && !errors.IsAlreadyExists(err) {
		msg := fmt.Sprintf("failed to create pod: %s error: %v", pod.Name, err)
		c.Recorder.Eventf(job, corev1.EventTypeWarning, FailedCreateReason, msg)
		return fmt.Errorf("failed to create pod: %v", err)
	}
	log.Info("create pod", "pod", pod)
	c.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulCreateReason, fmt.Sprintf("pod %v was successfully created", pod.Name))
	return nil
}

func (c *Context) CreateService(ctx context.Context, job *div2alpha1.DIJob, service *corev1.Service) error {
	log := c.Log.WithName("CreateService").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	if err := c.Create(ctx, service, &client.CreateOptions{}); err != nil && !errors.IsAlreadyExists(err) {
		msg := fmt.Sprintf("failed to create service: %s error: %v", service.Name, err)
		c.Recorder.Eventf(job, corev1.EventTypeWarning, FailedCreateReason, msg)
		return fmt.Errorf("failed to create service: %v", err)
	}
	log.Info("create service", "service", service)
	c.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulCreateReason, fmt.Sprintf("service %v was successfully created", service.Name))
	return nil
}

func (c *Context) DeletePodsAndServices(ctx context.Context, job *div2alpha1.DIJob, pods []*corev1.Pod, services []*corev1.Service) error {
	log := c.Log.WithName("DeletePodsAndServices").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	if len(pods) == 0 {
		return nil
	}
	for _, svc := range services {
		if err := c.DeleteService(ctx, job, svc); err != nil {
			return err
		}
	}
	if job.Spec.CleanPodPolicy != div2alpha1.CleanPodPolicyAll &&
		job.Spec.CleanPodPolicy != div2alpha1.CleanPodPolicyRunning {
		return nil
	}

	for _, pod := range pods {
		// Just delete running pod when the cleanPodPolicy is Running
		needsDelete := true
		if job.Spec.CleanPodPolicy == div2alpha1.CleanPodPolicyRunning {
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
		if diutil.IsPodTerminating(pod) {
			needsDelete = false
		}
		if !needsDelete {
			continue
		}

		msg := fmt.Sprintf("delete pod %s", pod.Name)
		log.Info(msg)
		if err := c.DeletePod(ctx, job, pod); err != nil {
			return err
		}
	}
	return nil
}

func (c *Context) DeletePods(ctx context.Context, job *div2alpha1.DIJob, pods []*corev1.Pod) error {
	var err error
	for _, pod := range pods {
		if err1 := c.DeletePod(ctx, job, pod); err != nil {
			err = err1
		}
	}
	return err
}

func (c *Context) DeletePod(ctx context.Context, job *div2alpha1.DIJob, pod *corev1.Pod) error {
	log := c.Log.WithName("DeletePod").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	if err := c.Delete(ctx, pod,
		&client.DeleteOptions{GracePeriodSeconds: func(a int64) *int64 { return &a }(0)}); err != nil && !errors.IsNotFound(err) {
		msg := fmt.Sprintf("failed to delete pod: %s error: %v", pod.Name, err)
		c.Recorder.Eventf(job, corev1.EventTypeWarning, FailedDeleteReason, msg)
		return fmt.Errorf("failed to delete pod: %v", err)
	}
	log.Info("delete pod", "pod", pod)
	c.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulDeleteReason, fmt.Sprintf("pod %v was successfully deleted", pod.Name))
	return nil
}

func (c *Context) DeleteService(ctx context.Context, job *div2alpha1.DIJob, service *corev1.Service) error {
	log := c.Log.WithName("DeleteService").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	if err := c.Delete(ctx, service,
		&client.DeleteOptions{GracePeriodSeconds: func(a int64) *int64 { return &a }(0)}); err != nil && !errors.IsNotFound(err) {
		msg := fmt.Sprintf("failed to delete service: %s error: %v", service.Name, err)
		c.Recorder.Eventf(job, corev1.EventTypeWarning, FailedDeleteReason, msg)
		return fmt.Errorf("failed to delete service: %v", err)
	}
	log.Info("delete service", "service", service)
	c.Recorder.Eventf(job, corev1.EventTypeNormal, SuccessfulDeleteReason, fmt.Sprintf("service %v was successfully deleted", service.Name))
	return nil
}

func (c *Context) ListJobPods(ctx context.Context, job *div2alpha1.DIJob) ([]*corev1.Pod, error) {
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: diutil.GenLabels(*job),
	})
	if err != nil {
		return nil, err
	}

	// list pods of job
	pods, err := c.ListPods(ctx, &client.ListOptions{Namespace: job.Namespace, LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}
	return pods, nil
}

func (c *Context) ListJobServices(ctx context.Context, job *div2alpha1.DIJob) ([]*corev1.Service, error) {
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: diutil.GenLabels(*job),
	})
	if err != nil {
		return nil, err
	}

	// list svcs of job
	svcs, err := c.ListServices(ctx, &client.ListOptions{Namespace: job.Namespace, LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}
	return svcs, nil
}

func (c *Context) ListPods(ctx context.Context, opts *client.ListOptions) ([]*corev1.Pod, error) {
	podList := &corev1.PodList{}
	err := c.List(ctx, podList, opts)
	if err != nil {
		return nil, err
	}

	pods := []*corev1.Pod{}
	for _, pod := range podList.Items {
		pods = append(pods, pod.DeepCopy())
	}
	return pods, nil
}

func (c *Context) ListServices(ctx context.Context, opts *client.ListOptions) ([]*corev1.Service, error) {
	svcList := &corev1.ServiceList{}
	err := c.List(ctx, svcList, opts)
	if err != nil {
		return nil, err
	}

	svcs := []*corev1.Service{}
	for _, pod := range svcList.Items {
		svcs = append(svcs, pod.DeepCopy())
	}
	return svcs, nil
}

func svcName(name string) string {
	return fmt.Sprintf("di-%s", name)
}
