package allocator

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ditypes "opendilab.org/di-orchestrator/pkg/allocator/types"
	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
	"opendilab.org/di-orchestrator/pkg/common"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

func getJobInfo(job *div2alpha1.DIJob) (ditypes.JobInfo, error) {
	res, err := getJobResources(job)
	if err != nil {
		return ditypes.JobInfo{}, err
	}
	jobinfo := ditypes.NewJobInfo(
		types.NamespacedName{
			Namespace: job.Namespace, Name: job.Name,
		},
		res, int(job.Spec.MinReplicas), int(job.Spec.MaxReplicas),
		job.Spec.Preemptible,
	)
	return *jobinfo, nil
}

func getJobResources(job *div2alpha1.DIJob) (corev1.ResourceRequirements, error) {
	res, err := common.GetDIJobDefaultResources()
	if err != nil {
		return corev1.ResourceRequirements{}, err
	}
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
	return res, nil
}
