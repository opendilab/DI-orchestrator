package controllers

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"opendilab.org/di-orchestrator/pkg/api/v2alpha1"
	dicontext "opendilab.org/di-orchestrator/pkg/context"
)

var (
	jobStatusHandlers map[v2alpha1.Phase]map[v2alpha1.Phase][]func(ctx dicontext.Context, job *v2alpha1.DIJob)
)

func init() {
	jobStatusHandlers = make(map[v2alpha1.Phase]map[v2alpha1.Phase][]func(ctx dicontext.Context, job *v2alpha1.DIJob))
	registerJobStatusHandlers()
}

func registerJobStatusHandlers() {
	registerEach := func(old, new v2alpha1.Phase, handlers ...func(ctx dicontext.Context, job *v2alpha1.DIJob)) {
		if jobStatusHandlers[old] == nil {
			jobStatusHandlers[old] = make(map[v2alpha1.Phase][]func(ctx dicontext.Context, job *v2alpha1.DIJob))
		}
		jobStatusHandlers[old][new] = handlers
	}

	registerEach(v2alpha1.JobPending, v2alpha1.JobPending, nothing)
	registerEach(v2alpha1.JobPending, v2alpha1.JobStarting, onJobStarting)
	registerEach(v2alpha1.JobPending, v2alpha1.JobRestarting, onJobRestarting, increaseRestartCount)
	registerEach(v2alpha1.JobPending, v2alpha1.JobRescheduling, onJobRescheduling, increaseRescheduleCount)
	registerEach(v2alpha1.JobPending, v2alpha1.JobRunning, onJobRunning)
	registerEach(v2alpha1.JobPending, v2alpha1.JobFailed, onJobFailed, updateReadyReplicas)
	registerEach(v2alpha1.JobPending, v2alpha1.JobSucceeded, onJobSucceeded, updateReadyReplicas)

	registerEach(v2alpha1.JobStarting, v2alpha1.JobPending, nothing)
	registerEach(v2alpha1.JobStarting, v2alpha1.JobStarting, nothing)
	registerEach(v2alpha1.JobStarting, v2alpha1.JobRestarting, onJobRestarting, increaseRestartCount)
	registerEach(v2alpha1.JobStarting, v2alpha1.JobRescheduling, onJobRescheduling, increaseRescheduleCount)
	registerEach(v2alpha1.JobStarting, v2alpha1.JobRunning, onJobRunning)
	registerEach(v2alpha1.JobStarting, v2alpha1.JobFailed, onJobFailed, updateReadyReplicas)
	registerEach(v2alpha1.JobStarting, v2alpha1.JobSucceeded, onJobSucceeded, updateReadyReplicas)

	registerEach(v2alpha1.JobRestarting, v2alpha1.JobPending, nothing)
	registerEach(v2alpha1.JobRestarting, v2alpha1.JobStarting, onJobStarting)
	registerEach(v2alpha1.JobRestarting, v2alpha1.JobRestarting, nothing)
	registerEach(v2alpha1.JobRestarting, v2alpha1.JobRescheduling, onJobRescheduling, increaseRescheduleCount)
	registerEach(v2alpha1.JobRestarting, v2alpha1.JobRunning, onJobRunning)
	registerEach(v2alpha1.JobRestarting, v2alpha1.JobFailed, onJobFailed, updateReadyReplicas)
	registerEach(v2alpha1.JobRestarting, v2alpha1.JobSucceeded, onJobSucceeded, updateReadyReplicas)

	registerEach(v2alpha1.JobRescheduling, v2alpha1.JobPending, nothing)
	registerEach(v2alpha1.JobRescheduling, v2alpha1.JobStarting, onJobStarting)
	registerEach(v2alpha1.JobRescheduling, v2alpha1.JobRestarting, onJobRestarting, increaseRestartCount)
	registerEach(v2alpha1.JobRescheduling, v2alpha1.JobRescheduling, nothing)
	registerEach(v2alpha1.JobRescheduling, v2alpha1.JobRunning, onJobRunning)
	registerEach(v2alpha1.JobRescheduling, v2alpha1.JobFailed, onJobFailed, updateReadyReplicas)
	registerEach(v2alpha1.JobRescheduling, v2alpha1.JobSucceeded, onJobSucceeded, updateReadyReplicas)

	registerEach(v2alpha1.JobRunning, v2alpha1.JobPending, nothing)
	registerEach(v2alpha1.JobRunning, v2alpha1.JobStarting, onJobStarting)
	registerEach(v2alpha1.JobRunning, v2alpha1.JobRestarting, onJobRestarting, increaseRestartCount)
	registerEach(v2alpha1.JobRunning, v2alpha1.JobRescheduling, onJobRescheduling, increaseRescheduleCount)
	registerEach(v2alpha1.JobRunning, v2alpha1.JobRunning, nothing)
	registerEach(v2alpha1.JobRunning, v2alpha1.JobFailed, onJobFailed, updateReadyReplicas)
	registerEach(v2alpha1.JobRunning, v2alpha1.JobSucceeded, onJobSucceeded, updateReadyReplicas)

	registerEach(v2alpha1.JobFailed, v2alpha1.JobPending, nothing)
	registerEach(v2alpha1.JobFailed, v2alpha1.JobStarting, onJobStarting)
	registerEach(v2alpha1.JobFailed, v2alpha1.JobRestarting, onJobRestarting, increaseRestartCount)
	registerEach(v2alpha1.JobFailed, v2alpha1.JobRescheduling, onJobRescheduling, increaseRescheduleCount)
	registerEach(v2alpha1.JobFailed, v2alpha1.JobRunning, onJobRunning)
	registerEach(v2alpha1.JobFailed, v2alpha1.JobFailed, nothing)
	registerEach(v2alpha1.JobFailed, v2alpha1.JobSucceeded, onJobSucceeded, updateReadyReplicas)

	registerEach(v2alpha1.JobSucceeded, v2alpha1.JobPending, nothing)
	registerEach(v2alpha1.JobSucceeded, v2alpha1.JobStarting, onJobStarting)
	registerEach(v2alpha1.JobSucceeded, v2alpha1.JobRestarting, onJobRestarting, increaseRestartCount)
	registerEach(v2alpha1.JobFailed, v2alpha1.JobRescheduling, onJobRescheduling, increaseRescheduleCount)
	registerEach(v2alpha1.JobSucceeded, v2alpha1.JobRunning, onJobRunning)
	registerEach(v2alpha1.JobSucceeded, v2alpha1.JobFailed, onJobFailed, updateReadyReplicas)
	registerEach(v2alpha1.JobSucceeded, v2alpha1.JobSucceeded, nothing)
}

func HandleJobStatus(ctx dicontext.Context, old, new *v2alpha1.DIJob) {
	for _, handler := range jobStatusHandlers[old.Status.Phase][new.Status.Phase] {
		handler(ctx, new)
	}
}

func nothing(ctx dicontext.Context, job *v2alpha1.DIJob) {}

func updateReadyReplicas(ctx dicontext.Context, job *v2alpha1.DIJob) {
	job.Status.ReadyReplicas = 0
}

func increaseRestartCount(ctx dicontext.Context, job *v2alpha1.DIJob) {
	job.Status.Restarts++
}

func increaseRescheduleCount(ctx dicontext.Context, job *v2alpha1.DIJob) {
	job.Status.Reschedules++
}

func onJobStarting(ctx dicontext.Context, job *v2alpha1.DIJob) {
	msg := "job is starting since all replicas are created."
	ctx.Recorder.Eventf(job, corev1.EventTypeNormal, dicontext.DIJobStartingReason, msg)
}

func onJobRunning(ctx dicontext.Context, job *v2alpha1.DIJob) {
	msg := "job is running since all replicas are ready."
	ctx.Recorder.Eventf(job, corev1.EventTypeNormal, dicontext.DIJobRunningReason, msg)
}

func onJobRestarting(ctx dicontext.Context, job *v2alpha1.DIJob) {
	msg := "job is restarting since conditions changed."
	ctx.Recorder.Eventf(job, corev1.EventTypeWarning, dicontext.DIJobRestartingReason, msg)
}

func onJobRescheduling(ctx dicontext.Context, job *v2alpha1.DIJob) {
	msg := "job is rescheduling since replicas or allocation changed."
	ctx.Recorder.Eventf(job, corev1.EventTypeWarning, dicontext.DIJobReschedulingReason, msg)
}

func onJobFailed(ctx dicontext.Context, job *v2alpha1.DIJob) {
	msg := "job is failed since some replicas are failed."
	ctx.Recorder.Eventf(job, corev1.EventTypeWarning, dicontext.DIJobFailedReason, msg)
	deleteTime := metav1.Now()
	job.Status.CompletionTimestamp = &deleteTime
}

func onJobSucceeded(ctx dicontext.Context, job *v2alpha1.DIJob) {
	msg := "job is succeeded since all the replicas are succeeded."
	ctx.Recorder.Eventf(job, corev1.EventTypeNormal, dicontext.DIJobSucceededReason, msg)
	deleteTime := metav1.Now()
	job.Status.CompletionTimestamp = &deleteTime
}
