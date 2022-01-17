package common

import (
	"encoding/json"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	// labels for pods
	LabelOperator = "diengine/operator"
	LabelGroup    = "diengine/group"
	LabelJob      = "diengine/job"
	LabelRank     = "diengine/rank"
	LabelPod      = "diengine/pod"

	// annotations for pods
	AnnotationGeneration = "diengine/generation"
	AnnotationReplicas   = "diengine/replicas"
	AnnotationRank       = "diengine/rank"
	AnnotationNode       = "diengine/node"

	// envs for pods
	ENVJobID              = "DI_JOB_ID"
	ENVJobGeneration      = "DI_JOB_GENERATION"
	ENVServerURL          = "DI_SERVER_URL"
	ENVParallelWorkersArg = "DI_PARALLEL_WORKERS_ARG"
	ENVPortsArg           = "DI_PORTS_ARG"
	ENVNodeIDsArg         = "DI_NODE_IDS_ARG"
	ENVAttachedNodesArg   = "DI_ATTACHED_NODES_ARG"

	// args for di-engine command
	DIArgParallelWorkers = "parallel-workers"
	DIArgPorts           = "ports"
	DIArgNodeIDs         = "node-ids"
	DIArgAttachedNodes   = "attach-to"
	DINodeURLPrefix      = "tcp://"

	// dijob oriented
	OperatorName         = "di-operator"
	DefaultContainerName = "di-container"
	DefaultPortName      = "di-port"
	DefaultPort          = 22270

	// system oriented
	ResourceGPU = "nvidia.com/gpu"
)

func GetDIServerURL() string {
	url := os.Getenv(ENVServerURL)
	if url == "" {
		return "http://di-server.di-system:8080"
	}
	return url
}

func GetDIJobDefaultResources() (corev1.ResourceRequirements, error) {
	defaultResource := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("1"),
		corev1.ResourceMemory: resource.MustParse("2Gi"),
	}
	resjson := os.Getenv("DI_JOB_DEFAULT_RESOURCES")
	if resjson == "" {
		return corev1.ResourceRequirements{Requests: defaultResource, Limits: defaultResource}, nil
	}
	resourceRequire := map[string]corev1.ResourceRequirements{}
	if err := json.Unmarshal([]byte(resjson), &resourceRequire); err != nil {
		return corev1.ResourceRequirements{}, fmt.Errorf("failed to unmarshal resource requirements: %v", err)
	}
	if _, ok := resourceRequire["resources"]; !ok {
		return corev1.ResourceRequirements{}, fmt.Errorf("failed to unmarshal resource requirements")
	}
	return resourceRequire["resources"], nil
}
