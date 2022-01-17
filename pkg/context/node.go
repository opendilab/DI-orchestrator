package context

import (
	corev1 "k8s.io/api/core/v1"
	resourcehelper "k8s.io/kubectl/pkg/util/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *Context) ListNodes() ([]*corev1.Node, error) {
	nodeList := corev1.NodeList{}
	if err := c.Client.List(c.ctx, &nodeList, &client.ListOptions{}); err != nil {
		return nil, err
	}
	nodes := []*corev1.Node{}
	for _, node := range nodeList.Items {
		nodes = append(nodes, node.DeepCopy())
	}
	return nodes, nil
}

func (c *Context) GetNodeAllocatedResources(node *corev1.Node, pods []*corev1.Pod) (reqs corev1.ResourceList, limits corev1.ResourceList, err error) {
	nonTerminatedPods := filterOutIneffectivePods(pods)
	reqs, limits = getPodsTotalRequestsAndLimits(nonTerminatedPods)
	return
}

func filterOutIneffectivePods(pods []*corev1.Pod) []*corev1.Pod {
	effectivePods := make([]*corev1.Pod, 0)
	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodUnknown || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		} else if pod.Status.Phase == corev1.PodPending {
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionTrue {
					effectivePods = append(effectivePods, pod)
				}
			}
		} else if pod.Status.Phase == corev1.PodRunning {
			effectivePods = append(effectivePods, pod)
		}
	}
	return effectivePods
}

func getPodsTotalRequestsAndLimits(podList []*corev1.Pod) (reqs corev1.ResourceList, limits corev1.ResourceList) {
	reqs, limits = corev1.ResourceList{}, corev1.ResourceList{}
	for _, pod := range podList {
		podReqs, podLimits := resourcehelper.PodRequestsAndLimits(pod)
		for podReqName, podReqValue := range podReqs {
			if value, ok := reqs[podReqName]; !ok {
				reqs[podReqName] = podReqValue.DeepCopy()
			} else {
				value.Add(podReqValue)
				reqs[podReqName] = value
			}
		}
		for podLimitName, podLimitValue := range podLimits {
			if value, ok := limits[podLimitName]; !ok {
				limits[podLimitName] = podLimitValue.DeepCopy()
			} else {
				value.Add(podLimitValue)
				limits[podLimitName] = value
			}
		}
	}
	return
}
