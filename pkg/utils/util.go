package util

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
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

func Int32(i int32) *int32 {
	return &i
}

func Bool(i bool) *bool {
	return &i
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

func ReplicaName(jobName string, taskName string, rank int) string {
	return fmt.Sprintf("%s-%s-%d", jobName, taskName, rank)
}

func PodFQDN(name, subdomain, namespace, domainName string) string {
	return fmt.Sprintf("%s.%s.%s.%s", name, subdomain, namespace, domainName)
}

func IsSucceeded(job *div2alpha1.DIJob) bool {
	return job.Status.Phase == div2alpha1.JobSucceeded
}

func IsFailed(job *div2alpha1.DIJob) bool {
	return job.Status.Phase == div2alpha1.JobFailed
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

func CountScheduledPods(pods []*corev1.Pod) int {
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
		if IsPodTerminating(pod) {
			continue
		}
		for _, c := range pod.Status.ContainerStatuses {
			if c.Ready {
				count++
			}
		}
	}
	return count
}

func CountCompletedPods(pods []*corev1.Pod, preemptible bool) (succeeded, failed int) {
	succeeded = 0
	failed = 0
	for _, pod := range pods {
		// replicas, _ := strconv.Atoi(pod.Annotations[dicommon.AnnotationReplicas])
		// if replicas == len(pods) && diutil.IsPodSucceeded(pod) {
		if IsPodSucceeded(pod) {
			succeeded++
			continue
		}
		if IsPodFailed(pod, preemptible) {
			failed++
		}
	}
	return succeeded, failed
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

func FilterPods(pods []*corev1.Pod, filters Filters) []*corev1.Pod {
	results := []*corev1.Pod{}
	for _, pod := range pods {
		if filters.Apply(pod) {
			results = append(results, pod)
		}
	}

	return results
}

func IsPodTerminating(pod *corev1.Pod) bool {
	return pod.DeletionTimestamp != nil
}

func IsPodSucceeded(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodSucceeded
}

func IsPodFailed(pod *corev1.Pod, preemptible bool) bool {
	exit143 := func(pod *corev1.Pod) bool {
		if pod.Status.ContainerStatuses == nil {
			return false
		}
		for _, status := range pod.Status.ContainerStatuses {
			if status.State.Terminated != nil && status.State.Terminated.ExitCode == 143 {
				return true
			}
		}
		return false
	}

	if pod.Status.Phase != corev1.PodUnknown && pod.Status.Phase != corev1.PodFailed {
		return false
	}
	if pod.Status.Reason == "UnexpectedAdmissionError" {
		// log.Info(fmt.Sprintf("pod %s UnexpectedAdmissionError occurred, message: %s", pod.Name, pod.Status.Message))
		return false
	} else if strings.HasPrefix(pod.Status.Reason, "Outof") {
		// log.Info(fmt.Sprintf("pod %s is %s on node %s", pod.Name, pod.Status.Reason, pod.Spec.NodeName))
		return false
	} else if preemptible && exit143(pod) {
		// log.Info(fmt.Sprintf("pod %s is terminated intentionally", pod.Name))
		return false
	} else if IsPodTerminating(pod) {
		// log.Info(fmt.Sprintf("pod %s has been deleted", pod.Name))
		return false
	}
	return true
}
