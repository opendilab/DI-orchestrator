package util

import (
	corev1 "k8s.io/api/core/v1"
)

type Filter func(obj interface{}) bool
type Filters []Filter

func (f Filters) Apply(obj interface{}) bool {
	for _, filter := range f {
		if !filter(obj) {
			return false
		}
	}
	return true
}

var (
	TerminatingPodFilter = func(obj interface{}) bool {
		pod := obj.(*corev1.Pod)
		return IsPodTerminating(pod)
	}

	NonTerminatingPodFilter = func(obj interface{}) bool {
		pod := obj.(*corev1.Pod)
		return !IsPodTerminating(pod)
	}
)
