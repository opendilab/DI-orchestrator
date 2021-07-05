package util

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div1alpha1 "go-sensephoenix.sensetime.com/di-orchestrator/api/v1alpha1"
)

const (
	randomLength = 5
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func GenerateName(name string) string {
	return fmt.Sprintf("%s-%s", name, utilrand.String(randomLength))
}

func GetObjectFromUnstructured(obj interface{}, dest interface{}) error {
	us, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("the object %s is not unstructured", obj)
	}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(us.UnstructuredContent(), dest)
	if err != nil {
		return err
	}

	return nil
}

func GetPortFromPod(pod *corev1.Pod) (int32, bool) {
	for _, c := range pod.Spec.Containers {
		if c.Name != DefaultContainerName {
			continue
		}
		for _, port := range c.Ports {
			if port.Name == DefaultPortName {
				return port.ContainerPort, true
			}
		}
	}
	return -1, false
}

func SetPortForPod(pod *corev1.Pod, port int32) {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name != DefaultContainerName {
			continue
		}
		if pod.Spec.Containers[i].Ports == nil {
			pod.Spec.Containers[i].Ports = []corev1.ContainerPort{}
		}
		portObj := corev1.ContainerPort{
			Name:          DefaultPortName,
			ContainerPort: port,
		}
		pod.Spec.Containers[i].Ports = append(pod.Spec.Containers[i].Ports, portObj)
	}
}

func GenLabels(jobName string) map[string]string {
	groupName := div1alpha1.GroupVersion.Group
	return map[string]string{
		GroupNameLabel:      groupName,
		JobNameLabel:        strings.Replace(jobName, "/", "-", -1),
		ControllerNameLabel: ControllerName,
	}
}

func AddLabelsToPod(pod *corev1.Pod, labels map[string]string) {
	if pod.ObjectMeta.Labels == nil {
		pod.ObjectMeta.Labels = make(map[string]string)
	}
	for k, v := range labels {
		pod.ObjectMeta.Labels[k] = v
	}
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
	case CollectorName:
		portEnv = "COLLECTOR_PORT"
		podName = GenerateName(podName)
	case LearnerName:
		portEnv = "LEARNER_PORT"
		podName = GenerateName(podName)
	case DDPLearnerName:
		portEnv = "LEARNER_PORT"
		podName = GenerateName(podName)
	case AggregatorName:
		portEnv = "AGGREGATOR_PORT"
		podName = GenerateName(podName)
	case CoordinatorName:
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
	labels[ReplicaTypeLabel] = replicaType
	labels[PodNameLabel] = pod.Name
	AddLabelsToPod(pod, labels)

	// get pod port
	port, ok := GetPortFromPod(pod)
	if !ok {
		port = defaultPort
		logrus.Infof("no port found, use default port for container %s port %d", DefaultContainerName, port)
		SetPortForPod(pod, port)
	}

	// add env
	envs := make(map[string]string)
	envs[portEnv] = fmt.Sprintf("%d", port)
	SetPodEnv(pod, envs)
	return pod, port, nil
}

func ReplicaPodName(name, replicaType string) string {
	return fmt.Sprintf("%s-%s", name, replicaType)
}

func SetPodEnv(pod *corev1.Pod, envs map[string]string) {
	// add env
	for i := range pod.Spec.Containers {
		if len(pod.Spec.Containers[i].Env) == 0 {
			pod.Spec.Containers[i].Env = make([]corev1.EnvVar, 0)
		}
		for k, v := range envs {
			env := corev1.EnvVar{
				Name:  k,
				Value: v,
			}
			pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, env)
		}
	}
}

func BuildService(labels map[string]string, port int32) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "None",
			Selector:  labels,
			Ports: []corev1.ServicePort{
				{
					Port: port,
					Name: DefaultPortName,
				},
			},
		},
	}

	return svc
}

func NamespacedName(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}

func SplitNamespaceName(namespaceName string) (types.NamespacedName, error) {
	strs := strings.Split(namespaceName, "/")
	if len(strs) != 2 {
		return types.NamespacedName{}, fmt.Errorf("Invalid namespace, name %s", namespaceName)
	}
	return types.NamespacedName{Namespace: strs[0], Name: strs[1]}, nil
}

func ConcatURL(name, ns string, port int32) string {
	return fmt.Sprintf("%s.%s:%d", name, ns, port)
}

func GetPodAccessURL(pod *corev1.Pod, defaultPort int32) string {
	port, found := GetPortFromPod(pod)
	if !found {
		port = defaultPort
	}
	return ConcatURL(pod.Name, pod.Namespace, port)
}

func GetServiceAccessURL(service *corev1.Service) string {
	url := ""
	for _, port := range service.Spec.Ports {
		if port.Name == DefaultPortName {
			url = ConcatURL(service.Name, service.Namespace, port.Port)
			break
		}
	}
	return url
}

func ListPods(ctx context.Context, cli client.Client, job *div1alpha1.DIJob) ([]*corev1.Pod, error) {
	podList := &corev1.PodList{}

	// generate label selector
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: GenLabels(job.Name),
	})
	if err != nil {
		return nil, err
	}

	// list pods of job
	err = cli.List(ctx, podList, &client.ListOptions{Namespace: job.Namespace, LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}

	pods := []*corev1.Pod{}
	for _, pod := range podList.Items {
		pods = append(pods, pod.DeepCopy())
	}
	return pods, nil
}

func ListServices(ctx context.Context, cli client.Client, job *div1alpha1.DIJob) ([]*corev1.Service, error) {
	svcList := &corev1.ServiceList{}

	// generate label selector
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: GenLabels(job.Name),
	})
	if err != nil {
		return nil, err
	}

	// list svcs of job
	err = cli.List(ctx, svcList, &client.ListOptions{Namespace: job.Namespace, LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}

	svcs := []*corev1.Service{}
	for _, svc := range svcList.Items {
		svcs = append(svcs, svc.DeepCopy())
	}
	return svcs, nil
}

func ClassifyPods(pods []*corev1.Pod) (collectors []*corev1.Pod, learners []*corev1.Pod,
	coordinator *corev1.Pod, aggregators []*corev1.Pod, DDPLearners []*corev1.Pod, err error) {
	// filter out collectors
	collectors, err = filterReplicaPods(pods, CollectorName)
	if err != nil {
		return
	}

	// filter out leader pods
	learners, err = filterReplicaPods(pods, LearnerName)
	if err != nil {
		return
	}

	// filter out coordinator pod
	coordinators, err := filterReplicaPods(pods, CoordinatorName)
	if err != nil {
		return
	}

	// filter aggregator pod
	aggregators, err = filterReplicaPods(pods, AggregatorName)
	if err != nil {
		return
	}

	DDPLearners, err = filterReplicaPods(pods, DDPLearnerName)
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

func FilterOutTerminatingPods(pods []*corev1.Pod) []*corev1.Pod {
	results := []*corev1.Pod{}
	for _, pod := range pods {
		if IsTerminating(pod) {
			continue
		}
		results = append(results, pod)
	}

	return results
}

// IsTerminating returns true if pod's DeletionTimestamp has been set
func IsTerminating(pod *corev1.Pod) bool {
	return pod.DeletionTimestamp != nil
}

func ClassifyServices(services []*corev1.Service) (collectors []*corev1.Service, learners []*corev1.Service,
	coordinator *corev1.Service, aggregators []*corev1.Service, DDPLearners []*corev1.Service, err error) {
	// filter out collectors
	collectors, err = filterReplicaServices(services, CollectorName)
	if err != nil {
		return
	}

	// filter out leader services
	learners, err = filterReplicaServices(services, LearnerName)
	if err != nil {
		return
	}

	// filter out coordinator service
	coordinators, err := filterReplicaServices(services, CoordinatorName)
	if err != nil {
		return
	}

	// filter aggregator service
	aggregators, err = filterReplicaServices(services, AggregatorName)
	if err != nil {
		return
	}

	DDPLearners, err = filterReplicaServices(services, DDPLearnerName)
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
		MatchLabels: map[string]string{ReplicaTypeLabel: replicaType},
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
