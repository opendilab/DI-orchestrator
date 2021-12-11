package http

import (
	"fmt"

	mapset "github.com/deckarep/golang-set"
	corev1 "k8s.io/api/core/v1"
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

func (s *DIServer) listReplicaPodsWithSelector(jobID string, labelSelector labels.Selector) (
	pods []*corev1.Pod, err error) {
	// list pods that belong to the DIJob
	id, err := diutil.SplitNamespaceName(jobID)
	if err != nil {
		return
	}
	pods, err = s.listPodsWithSelector(id.Namespace, labelSelector)
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
