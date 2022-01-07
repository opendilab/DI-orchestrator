package common

import (
	"os"
)

const (
	LabelOperator = "diengine/operator"
	LabelGroup    = "diengine/group"
	LabelJob      = "diengine/job"
	LabelRank     = "diengine/rank"
	LabelPod      = "diengine/pod"

	AnnotationGeneration = "diengine/generation"
	AnnotationReplicas   = "diengine/replicas"
	AnnotationRank       = "diengine/rank"
	AnnotationNode       = "diengine/node"

	OperatorName = "di-operator"

	DefaultContainerName = "di-container"
	DefaultPortName      = "di-port"
	DefaultPort          = 22270

	ENVJobID              = "DI_JOB_ID"
	ENVJobGeneration      = "DI_JOB_GENERATION"
	ENVServerURL          = "DI_SERVER_URL"
	ENVParallelWorkersArg = "DI_PARALLEL_WORKERS_ARG"
	ENVPortsArg           = "DI_PORTS_ARG"
	ENVNodeIDsArg         = "DI_NODE_IDS_ARG"
	ENVAttachedNodesArg   = "DI_ATTACHED_NODES_ARG"

	DIArgParallelWorkers = "parallel-workers"
	DIArgPorts           = "ports"
	DIArgNodeIDs         = "node-ids"
	DIArgAttachedNodes   = "attach-to"
	DINodeURLPrefix      = "tcp://"
)

func GetDIServerURL() string {
	url := os.Getenv(ENVServerURL)
	if url == "" {
		return "di-server.di-system:8080"
	}
	return url
}
