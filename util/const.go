package util

const (
	GroupNameLabel      = "group-name"
	JobNameLabel        = "nervexjob-name"
	ControllerNameLabel = "controller-name"
	ReplicaTypeLabel    = "replica-type"

	ControllerName  = "nervex-operator"
	ActorName       = "actor"
	LearnerName     = "learner"
	AggregatorName  = "aggregator"
	CoordinatorName = "coordinator"

	DefaultActorContainerName       = "actor"
	DefaultLearnerContainerName     = "learner"
	DefaultAggregatorContainerName  = "aggregator"
	DefaultCoordinatorContainerName = "coordinator"

	DefaultActorPortName       = "actor-port"
	DefaultLearnerPortName     = "learner-port"
	DefaultAggregatorPortName  = "aggregator-port"
	DefaultCoordinatorPortName = "coordinator-port"

	DefaultActorPort       = 22270
	DefaultLearnerPort     = 22271
	DefaultAggregatorPort  = 22272
	DefaultCoordinatorPort = 22273
)
