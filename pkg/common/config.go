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
	// AnnotationGeneration = "diengine/generation"
	AnnotationReplicas = "diengine/replicas"
	AnnotationRank     = "diengine/rank"
	AnnotationNode     = "diengine/node"
	AnnotationTaskType = "diengine/task-type"
	AnnotationTaskRank = "diengine/task-rank"

	// envs for orchestrator
	ENVDomainName = "K8S_SVC_DOMAIN_NAME"
	ENVServerURL  = "DI_SERVER_URL"
	// envs for pods
	ENVJobID = "DI_JOB_ID"
	ENVRank  = "DI_RANK"
	ENVNodes = "DI_NODES"

	// dijob oriented
	OperatorName         = "di-operator"
	DefaultContainerName = "di-container"
	DefaultPortName      = "di-port"
	DefaultPort          = 22270

	// system oriented
	ResourceGPU = "nvidia.com/gpu"
)

var (
	// k8s service domain name
	svcDomainName = "svc.cluster.local"

	// di server access url
	diServerURL = fmt.Sprintf("http://di-server.di-system.%s:8081", svcDomainName)
)

func GetDIServerURL() string {
	return diServerURL
}

func SetDIServerURL(serverURL string) {
	diServerURL = serverURL
}

func GetServiceDomainName() string {
	return svcDomainName
}

func SetServiceDomainName(domainName string) {
	svcDomainName = domainName
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
