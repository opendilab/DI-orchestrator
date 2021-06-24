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

	DefaultCollectorContainerName   = "collector"
	DefaultLearnerContainerName     = "learner"
	DefaultAggregatorContainerName  = "aggregator"
	DefaultCoordinatorContainerName = "coordinator"

	DefaultCollectorPortName   = "collector"
	DefaultLearnerPortName     = "learner"
	DefaultAggregatorPortName  = "aggregator"
	DefaultCoordinatorPortName = "coordinator"

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
)

var (
	DefaultServerURL = "nervex-server.nervex-system:8080"
)
