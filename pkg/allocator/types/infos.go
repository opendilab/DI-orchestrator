package types

import (
	apitypes "k8s.io/apimachinery/pkg/types"
)

type JobInfo struct {
	Key apitypes.NamespacedName
}

func NewJobInfo(key apitypes.NamespacedName) *JobInfo {
	return &JobInfo{Key: key}
}

type NodeInfo struct {
	Key string
}

func NewNodeInfo(key string) *NodeInfo {
	return &NodeInfo{Key: key}
}
