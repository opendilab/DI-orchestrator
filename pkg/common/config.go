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

	ENVJobID          = "DI_JOB_ID"
	ENVJobGeneration  = "DI_JOB_GENERATION"
	ENVServerURL      = "DI_SERVER_URL"
	ENVWorldSize      = "WORLD_SIZE"
	ENVLocalWorldSize = "LOCAL_WORLD_SIZE"
	ENVStartRank      = "START_RANK"
	ENVMasterAddr     = "MASTER_ADDR"
	ENVMasterPort     = "MASTER_PORT"
	DefaultMasterPort = 10314
)

func GetDIServerURL() string {
	url := os.Getenv(ENVServerURL)
	if url == "" {
		return "di-server.di-system:8080"
	} else {
		return url
	}
}
