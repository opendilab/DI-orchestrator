package http

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	servertypes "go-sensephoenix.sensetime.com/nervex-operator/server/types"
	nervexutil "go-sensephoenix.sensetime.com/nervex-operator/utils"
)

func (s *NerveXServer) getPodsByNames(namespace string, names []string) ([]*corev1.Pod, error) {
	var keys []string
	var pods []*corev1.Pod
	for _, name := range names {
		key := nervexutil.NamespacedName(namespace, name)
		keys = append(keys, key)
	}

	pods, err := s.getPodsByKeys(keys)
	if err != nil {
		return pods, err
	}
	return pods, nil
}

func (s *NerveXServer) getPodsByKeys(keys []string) ([]*corev1.Pod, error) {
	var pods []*corev1.Pod
	for _, key := range keys {
		pod, err := s.getPodByKey(key)
		if err != nil {
			return pods, err
		}
		pods = append(pods, pod)
	}
	return pods, nil
}

func (s *NerveXServer) getPodByKey(key string) (*corev1.Pod, error) {
	obj, exists, err := s.dyi.PodInformer.Informer().GetIndexer().GetByKey(key)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get pod: %s", err)
		return nil, fmt.Errorf(errMsg)
	}
	if !exists {
		errMsg := fmt.Sprintf("pod: %s not exists in cache", key)
		return nil, &servertypes.NerveXError{Type: servertypes.ErrorNotFound, Message: errMsg}
	}

	podUn := obj.(*unstructured.Unstructured)
	var pod corev1.Pod
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(podUn.UnstructuredContent(), &pod)
	if err != nil {
		errMsg := fmt.Sprintf("failed to convert unstructured: %s", podUn.UnstructuredContent())
		return nil, fmt.Errorf(errMsg)
	}

	return &pod, nil
}

func (s *NerveXServer) buildPodAndService(
	template *corev1.PodTemplateSpec,
	ownRefer metav1.OwnerReference,
	jobName string,
	namespace, replicaType, containerName, portName string,
	port int32,
	resources servertypes.ResourceQuantity,
	volumes []corev1.Volume) (*corev1.Pod, *corev1.Service, int32, error) {
	// build pod
	pod, port, err := nervexutil.BuildPodFromTemplate(template, ownRefer, jobName, namespace, replicaType, containerName, portName, port)
	if err != nil {
		return nil, nil, -1, err
	}
	// set pod resources
	s.setPodResources(pod, resources, containerName)

	// add volumes
	pod.Spec.Volumes = append(pod.Spec.Volumes, volumes...)

	// build service
	svc := nervexutil.BuildService(pod.GetLabels(), port, portName)
	svc.SetOwnerReferences([]metav1.OwnerReference{ownRefer})
	svc.Name = pod.Name
	return pod, svc, port, nil
}

func (s *NerveXServer) createPodAndService(namespace string, pod *corev1.Pod, service *corev1.Service) (*corev1.Pod, error) {
	// create pod
	newpod, err := s.createPod(namespace, pod)
	if err != nil {
		return nil, err
	}
	// create service
	if err := s.createService(namespace, service); err != nil {
		return newpod, err
	}
	return newpod, nil
}

func (s *NerveXServer) createPod(namespace string, pod *corev1.Pod) (*corev1.Pod, error) {
	newpod, err := s.KubeClient.CoreV1().Pods(namespace).Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return newpod, &servertypes.NerveXError{Type: servertypes.ErrorAlreadyExists, Message: err.Error()}
		}
		return nil, err
	}
	return newpod, nil
}

func (s *NerveXServer) createService(namespace string, service *corev1.Service) error {
	_, err := s.KubeClient.CoreV1().Services(namespace).Create(context.Background(), service, metav1.CreateOptions{})
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return &servertypes.NerveXError{Type: servertypes.ErrorAlreadyExists, Message: err.Error()}
		}
		return err
	}
	return nil
}

func (s *NerveXServer) deletePodAndService(namespace, name string) error {
	// delete pods
	if err := s.deletePod(namespace, name); err != nil {
		return err
	}

	// delete services
	if err := s.deleteService(namespace, name); err != nil {
		return err
	}
	return nil
}

func (s *NerveXServer) deletePod(namespace, name string) error {
	if err := s.KubeClient.CoreV1().Pods(namespace).Delete(context.Background(), name, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (s *NerveXServer) deleteService(namespace, name string) error {
	if err := s.KubeClient.CoreV1().Services(namespace).Delete(context.Background(), name, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (s *NerveXServer) setPodResources(pod *corev1.Pod, resources servertypes.ResourceQuantity, containerName string) {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name != containerName {
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

func (s *NerveXServer) getPodResources(pod *corev1.Pod, containerName string) servertypes.ResourceQuantity {
	resource := servertypes.ResourceQuantity{
		CPU:    resource.MustParse("0"),
		GPU:    resource.MustParse("0"),
		Memory: resource.MustParse("0"),
	}
	for _, container := range pod.Spec.Containers {
		if container.Name != containerName {
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

func (s *NerveXServer) listReplicaPodsWithSelector(namespace string, labelSelector labels.Selector) (
	collectors []*corev1.Pod, learners []*corev1.Pod,
	coordinator *corev1.Pod, aggregators []*corev1.Pod, DDPLearners []*corev1.Pod, err error) {
	// list pods that belong to the NerveXJob
	pods, err := s.listPodsWithSelector(namespace, labelSelector)
	if err != nil {
		return
	}

	// filter out terminating pods since these pods are deleted
	pods = nervexutil.FilterOutTerminatingPods(pods)

	// classify pods
	collectors, learners, coordinator, aggregators, DDPLearners, err = nervexutil.ClassifyPods(pods)
	if err != nil {
		return
	}
	return
}

func (s *NerveXServer) listPodsWithSelector(namespace string, labelSelector labels.Selector) ([]*corev1.Pod, error) {
	ret, err := s.dyi.PodInformer.Lister().ByNamespace(namespace).List(labelSelector)
	if err != nil {
		return nil, err
	}

	pods := []*corev1.Pod{}
	for _, obj := range ret {
		podUn := obj.(*unstructured.Unstructured)
		var pod corev1.Pod
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(podUn.UnstructuredContent(), &pod); err != nil {
			return nil, err
		}
		pods = append(pods, &pod)
	}

	return pods, nil
}
