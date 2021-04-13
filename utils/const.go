package util

const (
	GroupNameLabel      = "group-name"
	JobNameLabel        = "nervexjob-name"
	ControllerNameLabel = "controller-name"
	ReplicaTypeLabel    = "replica-type"
	PodNameLabel        = "pod-name"

	ControllerName  = "nervex-operator"
	ActorName       = "actor"
	LearnerName     = "learner"
	AggregatorName  = "aggregator"
	CoordinatorName = "coordinator"

	DefaultActorContainerName       = "actor"
	DefaultLearnerContainerName     = "learner"
	DefaultAggregatorContainerName  = "aggregator"
	DefaultCoordinatorContainerName = "coordinator"

	DefaultActorPortName       = "actor"
	DefaultLearnerPortName     = "learner"
	DefaultAggregatorPortName  = "aggregator"
	DefaultCoordinatorPortName = "coordinator"

	DefaultActorPort       = 22270
	DefaultLearnerPort     = 22271
	DefaultAggregatorPort  = 22272
	DefaultCoordinatorPort = 22273

	PodNamespaceEnv = "KUBERNETES_POD_NAMESPACE"
	PodNameEnv      = "KUBERNETES_POD_NAME"
)
