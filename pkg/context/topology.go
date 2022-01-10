package context

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"opendilab.org/di-orchestrator/pkg/api/v1alpha2"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

var (
	itoa = func(a []int) []string {
		str := make([]string, len(a))
		for i := 0; i < len(str); i++ {
			str[i] = fmt.Sprint(a[i])
		}
		return str
	}
)

func OnTopologyHandler(job *v1alpha2.DIJob, rank int, pod *corev1.Pod) {
	envs := make(map[string]string)
	pworkers := int(job.Spec.EngineFields.ParallelWorkers)
	ports := make([]int, pworkers)
	nodeIDs := make([]int, pworkers)
	for i := 0; i < pworkers; i++ {
		ports[i] = i + dicommon.DefaultPort
		nodeIDs[i] = i + pworkers*rank
	}

	buildURL := func(prefix, addr string, port int) string {
		return fmt.Sprintf("%s%s:%d", prefix, addr, port)
	}
	attachedNodesArgValue := ""
	switch job.Spec.EngineFields.Topology {
	case v1alpha2.TopologyAlone:
		// do nothing
	case v1alpha2.TopologyStar:
		worker0 := diutil.ReplicaName(job.Name, int(job.Status.Generation), 0)
		attachedNodesArgValue = fmt.Sprintf("--%s=%s", dicommon.DIArgAttachedNodes, buildURL(dicommon.DINodeURLPrefix, worker0, dicommon.DefaultPort))
	case v1alpha2.TopologyMesh:
		var nodes []string
		for i := 0; i < rank; i++ {
			workerName := diutil.ReplicaName(job.Name, int(job.Status.Generation), i)
			for j := 0; j < pworkers; j++ {
				node := buildURL(dicommon.DINodeURLPrefix, workerName, j+dicommon.DefaultPort)
				nodes = append(nodes, node)
			}
		}
		attachedNodesArgValue = fmt.Sprintf("--%s=%s", dicommon.DIArgAttachedNodes, strings.Join(nodes, ","))
	}

	envs[dicommon.ENVParallelWorkersArg] = fmt.Sprintf("--%s=%d", dicommon.DIArgParallelWorkers, pworkers)
	envs[dicommon.ENVPortsArg] = fmt.Sprintf("--%s=%s", dicommon.DIArgPorts, strings.Join(itoa(ports), ","))
	envs[dicommon.ENVNodeIDsArg] = fmt.Sprintf("--%s=%s", dicommon.DIArgNodeIDs, strings.Join(itoa(nodeIDs), ","))
	if rank == 0 {
		envs[dicommon.ENVAttachedNodesArg] = ""
	} else {
		envs[dicommon.ENVAttachedNodesArg] = attachedNodesArgValue
	}

	diutil.AddEnvsToPod(pod, envs)
}
