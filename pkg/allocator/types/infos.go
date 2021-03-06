package types

import (
	corev1 "k8s.io/api/core/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
)

type JobInfo struct {
	Key         apitypes.NamespacedName
	Resources   corev1.ResourceRequirements
	MinReplicas int
	MaxReplicas int
	Preemptible bool
}

func NewJobInfo(key apitypes.NamespacedName, r corev1.ResourceRequirements, minr int, maxr int, preemptible bool) *JobInfo {
	return &JobInfo{
		Key:         key,
		Resources:   r,
		MinReplicas: minr,
		MaxReplicas: maxr,
		Preemptible: preemptible,
	}
}

type NodeInfo struct {
	Key string
	// Resources is the list of the free resources on the node.
	Resources corev1.ResourceList
}

func NewNodeInfo(key string, r corev1.ResourceList) *NodeInfo {
	return &NodeInfo{Key: key, Resources: r}
}
