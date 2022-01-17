package context

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

func (c *Context) CleanUpJob(job *div1alpha2.DIJob) error {
	err := c.Delete(c.ctx, job, &client.DeleteOptions{})
	if err != nil {
		return err
	}
	time.Sleep(250 * time.Millisecond)

	pods, err := c.ListJobPods(job)
	if err != nil {
		return err
	}
	for _, pod := range pods {
		err = c.Delete(c.ctx, pod, &client.DeleteOptions{GracePeriodSeconds: func(a int64) *int64 { return &a }(0)})
		if err != nil {
			return err
		}
	}

	svcs, err := c.ListJobServices(job)
	if err != nil {
		return err
	}
	for _, svc := range svcs {
		err = c.Delete(c.ctx, svc, &client.DeleteOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Context) WaitForAllReplicas(job *div1alpha2.DIJob, phase corev1.PodPhase) error {
	if err := wait.Poll(100*time.Millisecond, 5*time.Minute, func() (bool, error) {
		pods, err := c.ListJobPods(job)
		if err != nil {
			return false, err
		}
		// if there are only coordinator, keep waiting
		if len(pods) <= 1 {
			return false, nil
		}
		for _, pod := range pods {
			if pod.Status.Phase != phase {
				return false, nil
			}
		}
		return true, nil
	}); err != nil {
		return err
	}

	return nil
}

var (
	itoa = func(a []int) []string {
		str := make([]string, len(a))
		for i := 0; i < len(str); i++ {
			str[i] = fmt.Sprint(a[i])
		}
		return str
	}
)

func OnTopologyHandler(job *div1alpha2.DIJob, rank int, pod *corev1.Pod) {
	envs := make(map[string]string)
	subdomain := job.Name
	pworkers := int(job.Spec.EngineFields.ParallelWorkers)
	ports := make([]int, pworkers)
	nodeIDs := make([]int, pworkers)
	for i := 0; i < pworkers; i++ {
		ports[i] = i + dicommon.DefaultPort
		nodeIDs[i] = i + pworkers*rank
	}

	buildURL := func(prefix, addr, subdomain string, port int) string {
		return fmt.Sprintf("%s%s.%s:%d", prefix, addr, subdomain, port)
	}
	buildArg := func(flag, value string) string {
		return fmt.Sprintf("--%s=%s", flag, value)
	}

	attachedNodesArgValue := ""
	switch job.Spec.EngineFields.Topology {
	case div1alpha2.TopologyAlone:
		// do nothing
	case div1alpha2.TopologyStar:
		worker0 := diutil.ReplicaName(job.Name, int(job.Status.Generation), 0)
		attachedNodesArgValue = buildArg(dicommon.DIArgAttachedNodes, buildURL(dicommon.DINodeURLPrefix, worker0, subdomain, dicommon.DefaultPort))
	case div1alpha2.TopologyMesh:
		var nodes []string
		for i := 0; i < rank; i++ {
			workerName := diutil.ReplicaName(job.Name, int(job.Status.Generation), i)
			for j := 0; j < pworkers; j++ {
				node := buildURL(dicommon.DINodeURLPrefix, workerName, subdomain, j+dicommon.DefaultPort)
				nodes = append(nodes, node)
			}
		}
		attachedNodesArgValue = buildArg(dicommon.DIArgAttachedNodes, strings.Join(nodes, ","))
	}

	envs[dicommon.ENVParallelWorkersArg] = buildArg(dicommon.DIArgParallelWorkers, fmt.Sprint(pworkers))
	envs[dicommon.ENVPortsArg] = buildArg(dicommon.DIArgPorts, strings.Join(itoa(ports), ","))
	envs[dicommon.ENVNodeIDsArg] = buildArg(dicommon.DIArgNodeIDs, strings.Join(itoa(nodeIDs), ","))
	if rank == 0 {
		envs[dicommon.ENVAttachedNodesArg] = ""
	} else {
		envs[dicommon.ENVAttachedNodesArg] = attachedNodesArgValue
	}

	diutil.AddEnvsToPod(pod, envs)
}
