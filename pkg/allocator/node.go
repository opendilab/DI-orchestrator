package allocator

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ditypes "opendilab.org/di-orchestrator/pkg/allocator/types"
)

func (a *Allocator) getNodeInfos(nodes []*corev1.Node) (map[string]*ditypes.NodeInfo, error) {
	nodeInfos := make(map[string]*ditypes.NodeInfo)
	// fieldSelector, err := fields.ParseSelector("spec.nodeName=" + name + ",status.phase!=" + string(corev1.PodSucceeded) + ",status.phase!=" + string(corev1.PodFailed))
	// fieldSelector := fields.SelectorFromSet(fields.Set{"spec.nodeName": name})
	// pods, err := c.ListPods(&client.ListOptions{FieldSelector: fieldSelector})
	pods, err := a.ctx.ListPods(&client.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, node := range nodes {
		nodePods := make([]*corev1.Pod, 0)
		for _, pod := range pods {
			if pod.Spec.NodeName == node.Name {
				nodePods = append(nodePods, pod)
			}
		}
		nodeInfo, err := a.getNodeInfo(node, nodePods)
		if err != nil {
			return nil, err
		}
		nodeInfos[node.Name] = nodeInfo
	}

	return nodeInfos, nil
}

func (a *Allocator) getNodeInfo(node *corev1.Node, pods []*corev1.Pod) (*ditypes.NodeInfo, error) {
	reqs, _, err := a.ctx.GetNodeAllocatedResources(node, pods)
	if err != nil {
		return nil, err
	}

	allocatable := node.Status.Allocatable
	free := corev1.ResourceList{}
	for resourceName, value := range allocatable {
		alloc := value.DeepCopy()
		alloc.Sub(reqs[resourceName])
		free[resourceName] = alloc
	}
	return &ditypes.NodeInfo{
		Key:       node.Name,
		Resources: free,
	}, nil
}
