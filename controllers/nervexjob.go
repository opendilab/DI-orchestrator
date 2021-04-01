package controllers

import (
	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
)

func isSucceeded(job *nervexv1alpha1.NerveXJob) bool {
	return job.Status.Phase == nervexv1alpha1.JobRunning
}

func isFailed(job *nervexv1alpha1.NerveXJob) bool {
	return job.Status.Phase == nervexv1alpha1.JobFailed
}
