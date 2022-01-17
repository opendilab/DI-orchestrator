package util

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"

	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
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

func NamespacedName(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}

func SplitNamespaceName(namespaceName string) (types.NamespacedName, error) {
	strs := strings.Split(namespaceName, "/")
	if len(strs) != 2 {
		return types.NamespacedName{}, fmt.Errorf("invalid namespace/name %s", namespaceName)
	}
	return types.NamespacedName{Namespace: strs[0], Name: strs[1]}, nil
}

func ReplicaName(jobName string, generation, rank int) string {
	return fmt.Sprintf("%s-%d-%d", jobName, generation, rank)
}

func IsSucceeded(job *div2alpha1.DIJob) bool {
	return job.Status.Phase == div2alpha1.JobSucceeded
}

func IsFailed(job *div2alpha1.DIJob) bool {
	return job.Status.Phase == div2alpha1.JobFailed
}

func IsTerminating(pod *corev1.Pod) bool {
	return pod.DeletionTimestamp != nil
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

func GenLabels(job div2alpha1.DIJob) map[string]string {
	return map[string]string{
		dicommon.LabelGroup:    job.Spec.Group,
		dicommon.LabelJob:      strings.Replace(job.Name, "/", "-", -1),
		dicommon.LabelOperator: dicommon.OperatorName,
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

func AddAnnotationsToPod(pod *corev1.Pod, annotations map[string]string) {
	if pod.ObjectMeta.Annotations == nil {
		pod.ObjectMeta.Annotations = make(map[string]string)
	}
	for k, v := range annotations {
		pod.ObjectMeta.Annotations[k] = v
	}
}

func AddEnvsToPod(pod *corev1.Pod, envs map[string]string) {
	for i := range pod.Spec.Containers {
		if len(pod.Spec.Containers[i].Env) == 0 {
			pod.Spec.Containers[i].Env = make([]corev1.EnvVar, 0)
		}
		for k, v := range envs {
			env := corev1.EnvVar{Name: k, Value: v}
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

func CountPodsScheduled(pods []*corev1.Pod) int {
	count := 0
	for _, pod := range pods {
		for _, c := range pod.Status.Conditions {
			if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionTrue {
				count++
			}
		}
	}
	return count
}

func CountReadyPods(pods []*corev1.Pod) int {
	count := 0
	for _, pod := range pods {
		for _, c := range pod.Status.ContainerStatuses {
			if c.Ready {
				count++
			}
		}
	}
	return count
}

func SetPodResources(pod *corev1.Pod, resources corev1.ResourceRequirements) {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name != dicommon.DefaultContainerName {
			continue
		}
		pod.Spec.Containers[i].Resources = resources
	}
}

func GetPodResources(spec *corev1.PodSpec) corev1.ResourceRequirements {
	for _, container := range spec.Containers {
		if container.Name != dicommon.DefaultContainerName {
			continue
		}
		return container.Resources
	}
	return corev1.ResourceRequirements{}
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

func NewOwnerReference(apiVersion, kind, name string, uid types.UID, controller bool) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
		UID:        uid,
		Controller: &controller,
	}
}
