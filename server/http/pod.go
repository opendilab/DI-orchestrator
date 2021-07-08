package http

import (
	"context"
	"fmt"
	"strings"

	mapset "github.com/deckarep/golang-set"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	servertypes "opendilab.org/di-orchestrator/server/types"
	diutil "opendilab.org/di-orchestrator/utils"
)

func (s *DIServer) getPodsByNames(namespace string, names []string) ([]*corev1.Pod, error) {
	// use set to filter out duplicate items
	nameSlice := []interface{}{}
	for _, name := range names {
		nameSlice = append(nameSlice, name)
	}
	nameSet := mapset.NewSetFromSlice(nameSlice)

	var keys []string
	var pods []*corev1.Pod
	for name := range nameSet.Iterator().C {
		key := diutil.NamespacedName(namespace, name.(string))
		keys = append(keys, key)
	}

	pods, err := s.getPodsByKeys(keys)
	if err != nil {
		return pods, err
	}
	return pods, nil
}

func (s *DIServer) getPodsByKeys(keys []string) ([]*corev1.Pod, error) {
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

func (s *DIServer) getPodByKey(key string) (*corev1.Pod, error) {
	obj, exists, err := s.dyi.PodInformer.Informer().GetIndexer().GetByKey(key)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get pod: %s", err)
		return nil, fmt.Errorf(errMsg)
	}
	if !exists {
		errMsg := fmt.Sprintf("pod: %s not exists in cache", key)
		return nil, &servertypes.DIError{Type: servertypes.ErrorNotFound, Message: errMsg}
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

func (s *DIServer) getServicesByNames(namespace string, names []string) ([]*corev1.Service, error) {
	var keys []string
	var services []*corev1.Service
	for _, name := range names {
		key := diutil.NamespacedName(namespace, name)
		keys = append(keys, key)
	}

	services, err := s.getServicesByKeys(keys)
	if err != nil {
		return services, err
	}
	return services, nil
}

func (s *DIServer) getServicesByKeys(keys []string) ([]*corev1.Service, error) {
	var services []*corev1.Service
	for _, key := range keys {
		svc, err := s.getServiceByKey(key)
		if err != nil {
			return services, err
		}
		services = append(services, svc)
	}
	return services, nil
}

func (s *DIServer) getServiceByKey(key string) (*corev1.Service, error) {
	obj, exists, err := s.dyi.ServiceInformer.Informer().GetIndexer().GetByKey(key)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get service: %s", err)
		return nil, fmt.Errorf(errMsg)
	}
	if !exists {
		errMsg := fmt.Sprintf("service: %s not exists in cache", key)
		return nil, &servertypes.DIError{Type: servertypes.ErrorNotFound, Message: errMsg}
	}

	serviceUn := obj.(*unstructured.Unstructured)
	var service corev1.Service
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(serviceUn.UnstructuredContent(), &service)
	if err != nil {
		errMsg := fmt.Sprintf("failed to convert unstructured: %s", serviceUn.UnstructuredContent())
		return nil, fmt.Errorf(errMsg)
	}
	return &service, nil
}

func (s *DIServer) buildPodAndService(
	template *corev1.PodTemplateSpec,
	ownRefer metav1.OwnerReference,
	jobName string,
	namespace, replicaType string,
	port int32,
	resources servertypes.ResourceQuantity,
	volumes []corev1.Volume) (*corev1.Pod, *corev1.Service, int32, error) {
	// build pod
	pod, port, err := diutil.BuildPodFromTemplate(template, ownRefer, jobName, namespace, replicaType, port)
	if err != nil {
		return nil, nil, -1, err
	}
	// set pod resources
	s.setPodResources(pod, resources)

	// add volumes
	pod.Spec.Volumes = append(pod.Spec.Volumes, volumes...)

	// build service
	svc := diutil.BuildService(pod.GetLabels(), port)
	svc.SetOwnerReferences([]metav1.OwnerReference{ownRefer})
	svc.Name = pod.Name
	return pod, svc, port, nil
}

func (s *DIServer) createPodAndService(namespace string, pod *corev1.Pod, service *corev1.Service) (*corev1.Pod, error) {
	// create pod
	newpod, err := s.createPod(namespace, pod)
	if err != nil {
		return nil, err
	}

	// make sure newpod is the controller of service
	for i := range service.OwnerReferences {
		service.OwnerReferences[i].Controller = func(c bool) *bool { return &c }(false)
	}
	ownRefer := metav1.OwnerReference{
		APIVersion: corev1.SchemeGroupVersion.Version,
		Kind:       "Pod",
		Name:       newpod.Name,
		UID:        newpod.UID,
		Controller: func(c bool) *bool { return &c }(true),
	}
	service.OwnerReferences = append(service.OwnerReferences, ownRefer)

	// create service
	if err := s.createService(namespace, service); err != nil {
		return newpod, err
	}
	return newpod, nil
}

func (s *DIServer) createPod(namespace string, pod *corev1.Pod) (*corev1.Pod, error) {
	newpod, err := s.KubeClient.CoreV1().Pods(namespace).Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return newpod, &servertypes.DIError{Type: servertypes.ErrorAlreadyExists, Message: err.Error()}
		}
		return nil, err
	}
	return newpod, nil
}

func (s *DIServer) createService(namespace string, service *corev1.Service) error {
	_, err := s.KubeClient.CoreV1().Services(namespace).Create(context.Background(), service, metav1.CreateOptions{})
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return &servertypes.DIError{Type: servertypes.ErrorAlreadyExists, Message: err.Error()}
		}
		return err
	}
	return nil
}

func (s *DIServer) deletePodAndService(namespace, name string) error {
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

func (s *DIServer) deletePod(namespace, name string) error {
	if err := s.KubeClient.CoreV1().Pods(namespace).Delete(context.Background(), name,
		metav1.DeleteOptions{GracePeriodSeconds: func(a int64) *int64 { return &a }(0)}); err != nil && !k8serrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (s *DIServer) deleteService(namespace, name string) error {
	if err := s.KubeClient.CoreV1().Services(namespace).Delete(context.Background(), name,
		metav1.DeleteOptions{GracePeriodSeconds: func(a int64) *int64 { return &a }(0)}); err != nil && !k8serrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (s *DIServer) setPodResources(pod *corev1.Pod, resources servertypes.ResourceQuantity) {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name != diutil.DefaultContainerName {
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

func (s *DIServer) getPodResources(pod *corev1.Pod, containerName string) servertypes.ResourceQuantity {
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

func (s *DIServer) listReplicaPodsWithSelector(namespace string, labelSelector labels.Selector) (
	collectors []*corev1.Pod, learners []*corev1.Pod,
	coordinator *corev1.Pod, aggregators []*corev1.Pod, DDPLearners []*corev1.Pod, err error) {
	// list pods that belong to the DIJob
	pods, err := s.listPodsWithSelector(namespace, labelSelector)
	if err != nil {
		return
	}

	// filter out terminating pods since these pods are deleted
	pods = diutil.FilterOutTerminatingPods(pods)

	// classify pods
	collectors, learners, coordinator, aggregators, DDPLearners, err = diutil.ClassifyPods(pods)
	if err != nil {
		return
	}
	return
}

func (s *DIServer) listPodsWithSelector(namespace string, labelSelector labels.Selector) ([]*corev1.Pod, error) {
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

func (s *DIServer) listReplicaServicesWithSelector(namespace string, labelSelector labels.Selector) (
	collectors []*corev1.Service, learners []*corev1.Service,
	coordinator *corev1.Service, aggregators []*corev1.Service, DDPLearners []*corev1.Service, err error) {
	// list services that belong to the DIJob
	services, err := s.listServicesWithSelector(namespace, labelSelector)
	if err != nil {
		return
	}

	// classify services
	collectors, learners, coordinator, aggregators, DDPLearners, err = diutil.ClassifyServices(services)
	if err != nil {
		return
	}
	return
}

func (s *DIServer) listServicesWithSelector(namespace string, labelSelector labels.Selector) ([]*corev1.Service, error) {
	ret, err := s.dyi.ServiceInformer.Lister().ByNamespace(namespace).List(labelSelector)
	if err != nil {
		return nil, err
	}

	services := []*corev1.Service{}
	for _, obj := range ret {
		serviceUn := obj.(*unstructured.Unstructured)
		var service corev1.Service
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(serviceUn.UnstructuredContent(), &service); err != nil {
			return nil, err
		}
		services = append(services, &service)
	}

	return services, nil
}

func rebuildPodAndService(oldPod *corev1.Pod, oldSvc *corev1.Service) (*corev1.Pod, *corev1.Service) {
	var pod *corev1.Pod = &corev1.Pod{}
	parts := strings.Split(oldPod.Name, "-")
	generateName := strings.Join(parts[:len(parts)-1], "-")
	name := diutil.GenerateName(generateName)

	pod.SetName(name)
	pod.SetOwnerReferences(oldPod.GetOwnerReferences())
	pod.Spec = oldPod.DeepCopy().Spec
	pod.Spec.NodeName = ""

	labels := oldPod.GetLabels()
	labels[diutil.PodNameLabel] = name
	diutil.AddLabelsToPod(pod, labels)

	// update pod env
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name != diutil.DefaultContainerName {
			continue
		}
		for j := range pod.Spec.Containers[i].Env {
			if pod.Spec.Containers[i].Env[j].Name == diutil.PodNameEnv {
				pod.Spec.Containers[i].Env[j].Value = pod.Name
			}
		}
	}

	// build service
	var svc *corev1.Service = &corev1.Service{}
	svc.SetName(name)
	svc.SetOwnerReferences(oldSvc.GetOwnerReferences())
	svc.Spec = oldSvc.DeepCopy().Spec
	svc.SetLabels(labels)

	return pod, svc
}
