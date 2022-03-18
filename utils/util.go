package util

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div1alpha1 "opendilab.org/di-orchestrator/api/v1alpha1"
	dicommon "opendilab.org/di-orchestrator/common"
	commontypes "opendilab.org/di-orchestrator/common/types"
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

func ReplicaPodName(name, replicaType string) string {
	return fmt.Sprintf("%s-%s", name, replicaType)
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

func GetDefaultPortFromPod(pod *corev1.Pod) (int32, bool) {
	for _, c := range pod.Spec.Containers {
		if c.Name != dicommon.DefaultContainerName {
			continue
		}
		for _, port := range c.Ports {
			if port.Name == dicommon.DefaultPortName {
				return port.ContainerPort, true
			}
		}
	}
	return -1, false
}

func AddPortToPod(pod *corev1.Pod, port corev1.ContainerPort) {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name != dicommon.DefaultContainerName {
			continue
		}
		if pod.Spec.Containers[i].Ports == nil {
			pod.Spec.Containers[i].Ports = []corev1.ContainerPort{}
		}
		pod.Spec.Containers[i].Ports = append(pod.Spec.Containers[i].Ports, port)
	}
}

func GenLabels(jobName string) map[string]string {
	groupName := div1alpha1.GroupVersion.Group
	return map[string]string{
		dicommon.GroupNameLabel:      groupName,
		dicommon.JobNameLabel:        strings.Replace(jobName, "/", "-", -1),
		dicommon.ControllerNameLabel: dicommon.ControllerName,
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

func AddEnvsToPod(pod *corev1.Pod, envs map[string]string) {
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

func GetEnvFromPod(pod *corev1.Pod, envName string) (string, bool) {
	for _, container := range pod.Spec.Containers {
		if container.Name != dicommon.DefaultContainerName {
			continue
		}
		for _, env := range container.Env {
			if env.Name == envName {
				return env.Value, true
			}
		}
	}
	return "", false
}

func BuildService(name, namespace string, ownRefer metav1.OwnerReference, labels map[string]string, port int32) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: []metav1.OwnerReference{ownRefer},
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "None",
			Selector:  labels,
			Ports: []corev1.ServicePort{
				{
					Port: port,
					Name: dicommon.DefaultPortName,
				},
			},
		},
	}

	return svc
}

func ConcatURL(name, ns string, port int32) string {
	return fmt.Sprintf("%s.%s:%d", name, ns, port)
}

func GetPodAccessURL(pod *corev1.Pod, defaultPort int32) string {
	port, found := GetDefaultPortFromPod(pod)
	if !found {
		port = defaultPort
	}
	return ConcatURL(pod.Name, pod.Namespace, port)
}

func GetServiceAccessURL(service *corev1.Service) string {
	url := ""
	for _, port := range service.Spec.Ports {
		if port.Name == dicommon.DefaultPortName {
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

func AddGPUPortsToPod(pod *corev1.Pod, total int, startPort int32) {
	for i := 1; i < total; i++ {
		pname := fmt.Sprintf("%s-%d", dicommon.DDPLearnerPortPrefix, i)
		pport := startPort + int32(i)
		port := corev1.ContainerPort{
			Name:          pname,
			ContainerPort: pport,
		}
		AddPortToPod(pod, port)
	}
}

func AddGPUPortsToService(service *corev1.Service, total int, startPort int32) {
	// gpu 0's port has already been created
	for i := 1; i < total; i++ {
		pname := fmt.Sprintf("%s-%d", dicommon.DDPLearnerPortPrefix, i)
		pport := startPort + int32(i)
		port := corev1.ServicePort{
			Name: pname,
			Port: pport,
		}
		service.Spec.Ports = append(service.Spec.Ports, port)
	}
}

func SetPodResources(pod *corev1.Pod, resources commontypes.ResourceQuantity) {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name != dicommon.DefaultContainerName {
			continue
		}
		if pod.Spec.Containers[i].Resources.Limits == nil {
			pod.Spec.Containers[i].Resources.Limits = make(corev1.ResourceList)
		}
		if pod.Spec.Containers[i].Resources.Requests == nil {
			pod.Spec.Containers[i].Resources.Requests = make(corev1.ResourceList)
		}

		// cpu and memory must not be zero
		if !resources.CPU.IsZero() {
			pod.Spec.Containers[i].Resources.Limits[corev1.ResourceCPU] = resources.CPU
			pod.Spec.Containers[i].Resources.Requests[corev1.ResourceCPU] = resources.CPU
		}
		if !resources.Memory.IsZero() {
			pod.Spec.Containers[i].Resources.Limits[corev1.ResourceMemory] = resources.Memory
			pod.Spec.Containers[i].Resources.Requests[corev1.ResourceMemory] = resources.Memory
		}
		if !resources.GPU.IsZero() {
			pod.Spec.Containers[i].Resources.Limits[corev1.ResourceName("nvidia.com/gpu")] = resources.GPU
			pod.Spec.Containers[i].Resources.Requests[corev1.ResourceName("nvidia.com/gpu")] = resources.GPU
		}
	}
}

func GetPodResources(pod *corev1.Pod) commontypes.ResourceQuantity {
	resource := commontypes.ResourceQuantity{
		CPU:    resource.MustParse("0"),
		GPU:    resource.MustParse("0"),
		Memory: resource.MustParse("0"),
	}
	for _, container := range pod.Spec.Containers {
		if container.Name != dicommon.DefaultContainerName {
			continue
		}
		if container.Resources.Limits == nil && container.Resources.Requests == nil {
			break
		}
		if container.Resources.Requests != nil {
			resource.CPU = container.Resources.Requests[corev1.ResourceCPU].DeepCopy()
			resource.GPU = container.Resources.Requests[corev1.ResourceName("nvidia.com/gpu")].DeepCopy()
			resource.Memory = container.Resources.Requests[corev1.ResourceMemory].DeepCopy()
		}
		if container.Resources.Limits != nil {
			resource.CPU = container.Resources.Limits[corev1.ResourceCPU].DeepCopy()
			resource.GPU = container.Resources.Limits[corev1.ResourceName("nvidia.com/gpu")].DeepCopy()
			resource.Memory = container.Resources.Limits[corev1.ResourceMemory].DeepCopy()
		}
	}
	return resource
}

func NewOwnerReference(apiVersion, kind, name string, uid types.UID, controller bool) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
		UID:        uid,
		Controller: &controller,
	}
}
