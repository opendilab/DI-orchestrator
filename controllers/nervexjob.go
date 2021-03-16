package controllers

import (
	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
)

var (
	GroupNameLabel      = "group-name"
	JobNameLabel        = "nervexjob-name"
	ControllerNameLabel = "controller-name"
	ReplicaTypeLabel    = "replica-type"

	ControllerName  = "nervex-operator"
	ActorName       = "actor"
	LearnerName     = "learner"
	CoordinatorName = "coordinator"
	AggregatorName  = "aggregator"
)

func isSucceeded(job *nervexv1alpha1.NervexJob) bool {
	return job.Status.Phase == nervexv1alpha1.JobRunning
}

func isFailed(job *nervexv1alpha1.NervexJob) bool {
	return job.Status.Phase == nervexv1alpha1.JobFailed
}
