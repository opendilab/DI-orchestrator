package util

import (
	"fmt"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	dicommon "opendilab.org/di-orchestrator/common"
)

func BuildPodAndService(
	template *corev1.PodTemplateSpec,
	ownRefer metav1.OwnerReference,
	jobName string,
	namespace, replicaType string,
	port int32,
	volumes []corev1.Volume) (*corev1.Pod, *corev1.Service, int32, error) {
	// build pod
	pod, port, err := BuildPodFromTemplate(template, ownRefer, jobName, namespace, replicaType, port)
	if err != nil {
		return nil, nil, -1, err
	}

	// add volumes
	pod.Spec.Volumes = append(pod.Spec.Volumes, volumes...)

	// add env
	envs := make(map[string]string)
	envs[dicommon.PodNamespaceEnv] = pod.Namespace
	envs[dicommon.PodNameEnv] = pod.Name
	envs[dicommon.ServerURLEnv] = dicommon.DefaultServerURL
	AddEnvsToPod(pod, envs)

	// build service
	svc := BuildService(pod.Name, namespace, ownRefer, pod.GetLabels(), port)
	return pod, svc, port, nil
}

func BuildPodFromTemplate(
	template *corev1.PodTemplateSpec,
	ownRefer metav1.OwnerReference,
	jobName string,
	ns, replicaType string,
	defaultPort int32) (*corev1.Pod, int32, error) {
	// generate name is the DIJob name
	portEnv := ""
	podName := ReplicaPodName(jobName, replicaType)
	switch replicaType {
	case dicommon.CollectorName:
		portEnv = "COLLECTOR_PORT"
		podName = GenerateName(podName)
	case dicommon.LearnerName:
		portEnv = "LEARNER_PORT"
		podName = GenerateName(podName)
	case dicommon.DDPLearnerName:
		portEnv = "LEARNER_PORT"
		podName = GenerateName(podName)
	case dicommon.AggregatorName:
		portEnv = "AGGREGATOR_PORT"
		podName = GenerateName(podName)
	case dicommon.CoordinatorName:
		portEnv = "COORDINATOR_PORT"
	default:
		return nil, -1, fmt.Errorf("wrong replica type: %s", replicaType)
	}

	// setup pod template
	template.SetName(podName)
	template.SetNamespace(ns)
	template.SetOwnerReferences([]metav1.OwnerReference{ownRefer})

	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		Spec:       *template.Spec.DeepCopy(),
		ObjectMeta: *template.ObjectMeta.DeepCopy(),
	}

	// add labels to pod
	labels := GenLabels(jobName)
	labels[dicommon.ReplicaTypeLabel] = replicaType
	labels[dicommon.PodNameLabel] = pod.Name
	AddLabelsToPod(pod, labels)

	// get pod port
	port, ok := GetDefaultPortFromPod(pod)
	if !ok {
		port = defaultPort
		logrus.Infof("no port found, use default port for container %s port %d", dicommon.DefaultContainerName, port)
		AddDefaultPortToPod(pod, port)
	}

	// add env
	envs := make(map[string]string)
	envs[portEnv] = fmt.Sprintf("%d", port)
	AddEnvsToPod(pod, envs)
	return pod, port, nil
}

func ClassifyPods(pods []*corev1.Pod) (collectors []*corev1.Pod, learners []*corev1.Pod,
	coordinator *corev1.Pod, aggregators []*corev1.Pod, DDPLearners []*corev1.Pod, err error) {
	// filter out collectors
	collectors, err = filterReplicaPods(pods, dicommon.CollectorName)
	if err != nil {
		return
	}

	// filter out leader pods
	learners, err = filterReplicaPods(pods, dicommon.LearnerName)
	if err != nil {
		return
	}

	// filter out coordinator pod
	coordinators, err := filterReplicaPods(pods, dicommon.CoordinatorName)
	if err != nil {
		return
	}

	// filter aggregator pod
	aggregators, err = filterReplicaPods(pods, dicommon.AggregatorName)
	if err != nil {
		return
	}

	DDPLearners, err = filterReplicaPods(pods, dicommon.DDPLearnerName)
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

func filterReplicaPods(pods []*corev1.Pod, replicaType string) ([]*corev1.Pod, error) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{dicommon.ReplicaTypeLabel: replicaType},
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

func ClassifyServices(services []*corev1.Service) (collectors []*corev1.Service, learners []*corev1.Service,
	coordinator *corev1.Service, aggregators []*corev1.Service, DDPLearners []*corev1.Service, err error) {
	// filter out collectors
	collectors, err = filterReplicaServices(services, dicommon.CollectorName)
	if err != nil {
		return
	}

	// filter out leader services
	learners, err = filterReplicaServices(services, dicommon.LearnerName)
	if err != nil {
		return
	}

	// filter out coordinator service
	coordinators, err := filterReplicaServices(services, dicommon.CoordinatorName)
	if err != nil {
		return
	}

	// filter aggregator service
	aggregators, err = filterReplicaServices(services, dicommon.AggregatorName)
	if err != nil {
		return
	}

	DDPLearners, err = filterReplicaServices(services, dicommon.DDPLearnerName)
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

func filterReplicaServices(services []*corev1.Service, replicaType string) ([]*corev1.Service, error) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{dicommon.ReplicaTypeLabel: replicaType},
	})
	if err != nil {
		return nil, err
	}

	result := []*corev1.Service{}
	for _, service := range services {
		if !selector.Matches(labels.Set(service.Labels)) {
			continue
		}
		result = append(result, service)
	}
	return result, nil
}
