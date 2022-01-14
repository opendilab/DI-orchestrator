package allocator

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ditypes "opendilab.org/di-orchestrator/pkg/allocator/types"
	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
	"opendilab.org/di-orchestrator/pkg/common"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

func getJobInfo(job *div1alpha2.DIJob) ditypes.JobInfo {
	res := getJobResources(job)
	jobinfo := ditypes.NewJobInfo(
		types.NamespacedName{
			Namespace: job.Namespace, Name: job.Name,
		},
		res, int(job.Spec.MinReplicas), int(job.Spec.MaxReplicas),
		job.Spec.Preemptible,
	)
	return *jobinfo
}

func getJobResources(job *div1alpha2.DIJob) corev1.ResourceRequirements {
	res := common.GetDIJobDefaultResources()
	jobres := diutil.GetPodResources(&job.Spec.Template.Spec)
	if jobres.Requests != nil {
		if jobres.Requests.Cpu() != nil {
			res.Requests[corev1.ResourceCPU] = *jobres.Requests.Cpu()
		}
		if jobres.Requests.Memory() != nil {
			res.Requests[corev1.ResourceMemory] = *jobres.Requests.Memory()
		}
		res.Requests[corev1.ResourceName(common.ResourceGPU)] = jobres.Requests[corev1.ResourceName(common.ResourceGPU)]
	} else if jobres.Limits != nil {
		if jobres.Limits.Cpu() != nil {
			res.Limits[corev1.ResourceCPU] = *jobres.Limits.Cpu()
		}
		if jobres.Limits.Memory() != nil {
			res.Limits[corev1.ResourceMemory] = *jobres.Limits.Memory()
		}
		res.Limits[corev1.ResourceName(common.ResourceGPU)] = jobres.Limits[corev1.ResourceName(common.ResourceGPU)]
	}
	if _, ok := res.Requests[corev1.ResourceName(common.ResourceGPU)]; !ok {
		res.Requests[corev1.ResourceName(common.ResourceGPU)] = res.Limits[corev1.ResourceName(common.ResourceGPU)]
	}
	return res
}
