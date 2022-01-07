package handler

import (
	corev1 "k8s.io/api/core/v1"

	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
)

type JobStatusHandler func(job *div1alpha2.DIJob, pods []*corev1.Pod) error
