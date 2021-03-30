package util

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
)

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

func GetPortFromPodTemplate(template *corev1.PodTemplateSpec, containerName, portName string) (int32, bool) {
	for _, c := range template.Spec.Containers {
		if c.Name != containerName {
			continue
		}
		for _, port := range c.Ports {
			if port.Name == portName {
				return port.ContainerPort, true
			}
		}
	}
	return -1, false
}

func SetPodTemplatePort(template *corev1.PodTemplateSpec, containerName, portName string, port int32) {
	for i := range template.Spec.Containers {
		if template.Spec.Containers[i].Name != containerName {
			continue
		}
		if template.Spec.Containers[i].Ports == nil {
			template.Spec.Containers[i].Ports = []corev1.ContainerPort{}
		}
		portObj := corev1.ContainerPort{
			Name:          portName,
			ContainerPort: port,
		}
		template.Spec.Containers[i].Ports = append(template.Spec.Containers[i].Ports, portObj)
	}
}

func GenLabels(jobName string) map[string]string {
	groupName := nervexv1alpha1.GroupVersion.Group
	return map[string]string{
		GroupNameLabel:      groupName,
		JobNameLabel:        strings.Replace(jobName, "/", "-", -1),
		ControllerNameLabel: ControllerName,
	}
}

func AddLabelsToPodTemplate(podTemplate *corev1.PodTemplateSpec, labels map[string]string) {
	if podTemplate.ObjectMeta.Labels == nil {
		podTemplate.ObjectMeta.Labels = make(map[string]string)
	}
	for k, v := range labels {
		podTemplate.ObjectMeta.Labels[k] = v
	}
}

func BuildPodFromTemplate(template *corev1.PodTemplateSpec) *corev1.Pod {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
	}
	pod.Spec = *template.Spec.DeepCopy()
	pod.ObjectMeta = *template.ObjectMeta.DeepCopy()
	return pod
}

func SetPodTemplateEnv(template *corev1.PodTemplateSpec, envs map[string]string) {
	// add env
	for i := range template.Spec.Containers {
		if len(template.Spec.Containers[i].Env) == 0 {
			template.Spec.Containers[i].Env = make([]corev1.EnvVar, 0)
		}
		for k, v := range envs {
			env := corev1.EnvVar{
				Name:  k,
				Value: v,
			}
			template.Spec.Containers[i].Env = append(template.Spec.Containers[i].Env, env)
		}
	}
}

func BuildService(labels map[string]string, port int32, portName string) *corev1.Service {
	return &corev1.Service{
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Selector:  labels,
			Ports: []corev1.ServicePort{
				{
					Port: port,
					Name: portName,
				},
			},
		},
	}
}
