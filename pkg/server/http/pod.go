package http

import (
	"context"
	"fmt"

	mapset "github.com/deckarep/golang-set"
	corev1 "k8s.io/api/core/v1"
	errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	commontypes "opendilab.org/di-orchestrator/pkg/common/types"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
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
		return nil, &commontypes.DIError{Type: commontypes.ErrorNotFound, Message: errMsg}
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

func (s *DIServer) createPod(namespace string, pod *corev1.Pod) (*corev1.Pod, error) {
	newpod, err := s.KubeClient.CoreV1().Pods(namespace).Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, err
	}
	return newpod, nil
}

func (s *DIServer) deletePods(pods []*corev1.Pod) error {
	for _, pod := range pods {
		// delete old pod and service
		if err := s.deletePod(pod.Namespace, pod.Name); err != nil {
			return err
		}
	}
	return nil
}

func (s *DIServer) deletePod(namespace, name string) error {
	if err := s.KubeClient.CoreV1().Pods(namespace).Delete(context.Background(), name,
		metav1.DeleteOptions{GracePeriodSeconds: func(a int64) *int64 { return &a }(0)}); err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
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
