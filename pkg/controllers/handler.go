package controllers

import (
	corev1 "k8s.io/api/core/v1"
	"opendilab.org/di-orchestrator/pkg/api/v1alpha2"
	dicontext "opendilab.org/di-orchestrator/pkg/context"
)

var (
	jobStatusHandlers map[v1alpha2.Phase]map[v1alpha2.Phase][]func(ctx dicontext.Context, job *v1alpha2.DIJob)
)

func init() {
	jobStatusHandlers = make(map[v1alpha2.Phase]map[v1alpha2.Phase][]func(ctx dicontext.Context, job *v1alpha2.DIJob))
	registerJobStatusHandlers()
}

func registerJobStatusHandlers() {
	registerEach := func(old, new v1alpha2.Phase, handlers ...func(ctx dicontext.Context, job *v1alpha2.DIJob)) {
		if jobStatusHandlers[old] == nil {
			jobStatusHandlers[old] = make(map[v1alpha2.Phase][]func(ctx dicontext.Context, job *v1alpha2.DIJob))
		}
		jobStatusHandlers[old][new] = handlers
	}

	registerEach(v1alpha2.JobPending, v1alpha2.JobPending, nothing)
	registerEach(v1alpha2.JobPending, v1alpha2.JobStarting, onJobStarting)
	registerEach(v1alpha2.JobPending, v1alpha2.JobRestarting, onJobRestarting, updateGeneration)
	registerEach(v1alpha2.JobPending, v1alpha2.JobRunning, onJobRunning)
	registerEach(v1alpha2.JobPending, v1alpha2.JobFailed, onJobFailed, updateReadyReplicas)
	registerEach(v1alpha2.JobPending, v1alpha2.JobSucceeded, onJobSucceeded, updateReadyReplicas)

	registerEach(v1alpha2.JobStarting, v1alpha2.JobPending, nothing)
	registerEach(v1alpha2.JobStarting, v1alpha2.JobStarting, nothing)
	registerEach(v1alpha2.JobStarting, v1alpha2.JobRestarting, onJobRestarting, updateGeneration)
	registerEach(v1alpha2.JobStarting, v1alpha2.JobRunning, onJobRunning)
	registerEach(v1alpha2.JobStarting, v1alpha2.JobFailed, onJobFailed, updateReadyReplicas)
	registerEach(v1alpha2.JobStarting, v1alpha2.JobSucceeded, onJobSucceeded, updateReadyReplicas)

	registerEach(v1alpha2.JobRestarting, v1alpha2.JobPending, nothing)
	registerEach(v1alpha2.JobRestarting, v1alpha2.JobStarting, onJobStarting)
	registerEach(v1alpha2.JobRestarting, v1alpha2.JobRestarting, nothing)
	registerEach(v1alpha2.JobRestarting, v1alpha2.JobRunning, onJobRunning)
	registerEach(v1alpha2.JobRestarting, v1alpha2.JobFailed, onJobFailed, updateReadyReplicas)
	registerEach(v1alpha2.JobRestarting, v1alpha2.JobSucceeded, onJobSucceeded, updateReadyReplicas)

	registerEach(v1alpha2.JobRunning, v1alpha2.JobPending, nothing)
	registerEach(v1alpha2.JobRunning, v1alpha2.JobStarting, onJobStarting)
	registerEach(v1alpha2.JobRunning, v1alpha2.JobRestarting, onJobRestarting, updateGeneration)
	registerEach(v1alpha2.JobRunning, v1alpha2.JobRunning, nothing)
	registerEach(v1alpha2.JobRunning, v1alpha2.JobFailed, onJobFailed, updateReadyReplicas)
	registerEach(v1alpha2.JobRunning, v1alpha2.JobSucceeded, onJobSucceeded, updateReadyReplicas)

	registerEach(v1alpha2.JobFailed, v1alpha2.JobPending, nothing)
	registerEach(v1alpha2.JobFailed, v1alpha2.JobStarting, onJobStarting)
	registerEach(v1alpha2.JobFailed, v1alpha2.JobRestarting, onJobRestarting, updateGeneration)
	registerEach(v1alpha2.JobFailed, v1alpha2.JobRunning, onJobRunning)
	registerEach(v1alpha2.JobFailed, v1alpha2.JobFailed, nothing)
	registerEach(v1alpha2.JobFailed, v1alpha2.JobSucceeded, onJobSucceeded, updateReadyReplicas)

	registerEach(v1alpha2.JobSucceeded, v1alpha2.JobPending, nothing)
	registerEach(v1alpha2.JobSucceeded, v1alpha2.JobStarting, onJobStarting)
	registerEach(v1alpha2.JobSucceeded, v1alpha2.JobRestarting, onJobRestarting, updateGeneration)
	registerEach(v1alpha2.JobSucceeded, v1alpha2.JobRunning, onJobRunning)
	registerEach(v1alpha2.JobSucceeded, v1alpha2.JobFailed, onJobFailed, updateReadyReplicas)
	registerEach(v1alpha2.JobSucceeded, v1alpha2.JobSucceeded, nothing)
}

func HandleJobStatus(ctx dicontext.Context, old, new *v1alpha2.DIJob) {
	for _, handler := range jobStatusHandlers[old.Status.Phase][new.Status.Phase] {
		handler(ctx, new)
	}
}

func nothing(ctx dicontext.Context, job *v1alpha2.DIJob) {}

func updateReadyReplicas(ctx dicontext.Context, job *v1alpha2.DIJob) {
	job.Status.ReadyReplicas = 0
}

func updateGeneration(ctx dicontext.Context, job *v1alpha2.DIJob) {
	job.Status.Generation++
}

func onJobStarting(ctx dicontext.Context, job *v1alpha2.DIJob) {
	msg := "job is starting since all replicas are created."
	ctx.Recorder.Eventf(job, corev1.EventTypeNormal, dicontext.DIJobStartingReason, msg)
}

func onJobRunning(ctx dicontext.Context, job *v1alpha2.DIJob) {
	msg := "job is running since all replicas are ready."
	ctx.Recorder.Eventf(job, corev1.EventTypeNormal, dicontext.DIJobRunningReason, msg)
}

func onJobRestarting(ctx dicontext.Context, job *v1alpha2.DIJob) {
	msg := "job is restarting since conditions changed."
	ctx.Recorder.Eventf(job, corev1.EventTypeWarning, dicontext.DIJobRestartingReason, msg)
}

func onJobFailed(ctx dicontext.Context, job *v1alpha2.DIJob) {
	msg := "job is failed since some replicas are failed."
	ctx.Recorder.Eventf(job, corev1.EventTypeWarning, dicontext.DIJobFailedReason, msg)
}

func onJobSucceeded(ctx dicontext.Context, job *v1alpha2.DIJob) {
	msg := "job is succeeded since all the replicas are succeeded."
	ctx.Recorder.Eventf(job, corev1.EventTypeNormal, dicontext.DIJobSucceededReason, msg)
}
