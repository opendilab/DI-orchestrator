package util

const (
	GroupNameLabel      = "group-name"
	JobNameLabel        = "nervexjob-name"
	ControllerNameLabel = "controller-name"
	ReplicaTypeLabel    = "replica-type"
	PodNameLabel        = "pod-name"

	ControllerName  = "nervex-operator"
	CollectorName   = "collector"
	LearnerName     = "learner"
	DDPLearnerName  = "ddp-learner"
	AggregatorName  = "aggregator"
	CoordinatorName = "coordinator"

	DefaultContainerName = "nervex-container"

	DefaultPortName = "nervex-port"

	DefaultCollectorPort   = 22270
	DefaultLearnerPort     = 22271
	DefaultAggregatorPort  = 22272
	DefaultCoordinatorPort = 22273

	DDPLearnerPortPrefix = "gpu-port"

	PodNamespaceEnv   = "KUBERNETES_POD_NAMESPACE"
	PodNameEnv        = "KUBERNETES_POD_NAME"
	CoordinatorURLEnv = "KUBERNETES_COORDINATOR_URL"
	AggregatorURLEnv  = "KUBERNETES_AGGREGATOR_URL"
	ServerURLEnv      = "KUBERNETES_SERVER_URL"

	WorldSize         = "WORLD_SIZE"
	LocalWorldSize    = "LOCAL_WORLD_SIZE"
	StartRank         = "START_RANK"
	MasterAddr        = "MASTER_ADDR"
	MasterPort        = "MASTER_PORT"
	DefaultMasterPort = 10314
)

var (
	DefaultServerURL = "nervex-server.nervex-system:8080"
)
