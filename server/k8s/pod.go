package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"

	"go-sensephoenix.sensetime.com/nervex-operator/server/dynamic"
	servertypes "go-sensephoenix.sensetime.com/nervex-operator/server/types"
	nervexutil "go-sensephoenix.sensetime.com/nervex-operator/utils"
)

func GetPodByKey(dyi dynamic.Informers, key string) (*corev1.Pod, error) {
	obj, exists, err := dyi.PodInformer.Informer().GetIndexer().GetByKey(key)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get pod: %s", err)
		return nil, fmt.Errorf(errMsg)
	}
	if !exists {
		errMsg := fmt.Sprintf("pod: %s not exists in cache", key)
		return nil, &servertypes.NerveXError{Type: servertypes.ErrorNotFound, Message: errMsg}
	}

	podUn := obj.(*unstructured.Unstructured)
	var pod corev1.Pod
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(podUn.UnstructuredContent(), &pod)
	if err != nil {
		errMsg := fmt.Sprintf("failed to convert unstructured: %s", podUn.UnstructuredContent())
		return nil, fmt.Errorf(errMsg)
	}

	return &pod, nil
}

func CreatePodsAndServices(
	kubeClient *kubernetes.Clientset,
	template *corev1.PodTemplateSpec,
	ownRefer metav1.OwnerReference,
	njreq *servertypes.NerveXJobRequest,
	replicaType, containerName, portName string, defaultPort int32) ([]string, error) {

	ns := njreq.Namespace
	resources := servertypes.ResourceQuantity{}
	switch replicaType {
	case nervexutil.CollectorName:
		resources = njreq.Collectors
	case nervexutil.LearnerName:
		resources = njreq.Learners
	}

	results := []string{}
	// create pods and services
	for i := 0; i < resources.Replicas; i++ {
		// build pod
		pod, port, err := nervexutil.BuildPodFromTemplate(template.DeepCopy(), ownRefer, ns, replicaType, containerName, portName, defaultPort)
		if err != nil {
			return results, err
		}
		// set pod resources
		SetPodResources(pod, resources, containerName)

		// create pod
		_, err = kubeClient.CoreV1().Pods(ns).Create(context.Background(), pod, metav1.CreateOptions{})
		if err != nil {
			if k8serrors.IsAlreadyExists(err) {

			}
			return results, err
		}

		// build service
		svc := nervexutil.BuildService(pod.GetLabels(), port, portName)
		svc.SetOwnerReferences([]metav1.OwnerReference{ownRefer})
		svc.Name = pod.Name

		// create service
		_, err = kubeClient.CoreV1().Services(ns).Create(context.Background(), svc, metav1.CreateOptions{})
		if err != nil {
			if k8serrors.IsAlreadyExists(err) {

			}
			return results, err
		}

		result := nervexutil.ConcatURL(svc.Name, ns, port)
		results = append(results, result)
	}

	return results, nil
}

func ListReplicaPodsWithSelector(dyi dynamic.Informers, ns string, labelSelector labels.Selector) (
	collectors []*corev1.Pod, learners []*corev1.Pod, coordinator *corev1.Pod, aggregator *corev1.Pod, err error) {
	// list pods that belong to the NerveXJob
	pods, err := ListPodsWithSelector(dyi, ns, labelSelector)
	if err != nil {
		return
	}

	// filter out terminating pods since these pods are deleted
	pods = FilterOutTerminatingPods(pods)

	// classify pods
	collectors, learners, coordinator, aggregator, err = nervexutil.ClassifyPods(pods)
	if err != nil {
		return
	}
	return
}

func ListPodsWithSelector(dyi dynamic.Informers, namespace string, labelSelector labels.Selector) ([]*corev1.Pod, error) {
	ret, err := dyi.PodInformer.Lister().ByNamespace(namespace).List(labelSelector)
	if err != nil {
		return nil, err
	}

	pods := []*corev1.Pod{}
	for _, obj := range ret {
		podUn := obj.(*unstructured.Unstructured)
		var pod corev1.Pod
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(podUn.UnstructuredContent(), &pod); err != nil {
			return nil, err
		}
		pods = append(pods, &pod)
	}

	return pods, nil
}

func FilterOutTerminatingPods(pods []*corev1.Pod) []*corev1.Pod {
	results := []*corev1.Pod{}
	for _, pod := range pods {
		if isTerminating(pod) {
			continue
		}
		results = append(results, pod)
	}

	return results
}

func DeletePodsAndServices(kubeClient *kubernetes.Clientset, pods []*corev1.Pod, njreq *servertypes.NerveXJobRequest, replicaType string) ([]string, error) {
	results := []string{}
	resources := servertypes.ResourceQuantity{}
	var containerName, portName string
	var defaultPort int32
	ns := njreq.Namespace

	switch replicaType {
	case nervexutil.CollectorName:
		resources = njreq.Collectors
		containerName = nervexutil.DefaultCollectorContainerName
		portName = nervexutil.DefaultCollectorPortName
		defaultPort = nervexutil.DefaultCollectorPort
	case nervexutil.LearnerName:
		resources = njreq.Learners
		containerName = nervexutil.DefaultLearnerContainerName
		portName = nervexutil.DefaultLearnerPortName
		defaultPort = nervexutil.DefaultLearnerPort
	default:

	}

	for _, pod := range pods {
		// break if enough
		if len(results) >= resources.Replicas {
			break
		}

		// delete pods
		err := kubeClient.CoreV1().Pods(njreq.Namespace).Delete(context.Background(), pod.Name, metav1.DeleteOptions{})
		if err != nil {
			if k8serrors.IsNotFound(err) {

			}
			return results, err
		}

		// delete services
		err = kubeClient.CoreV1().Services(njreq.Namespace).Delete(context.Background(), pod.Name, metav1.DeleteOptions{})
		if err != nil {
			if k8serrors.IsNotFound(err) {

			}
			return results, err
		}

		result := nervexutil.GetPodAccessURL(pod, ns, containerName, portName, defaultPort)
		results = append(results, result)
	}

	return results, nil
}

// isTerminating returns true if pod's DeletionTimestamp has been set
func isTerminating(pod *corev1.Pod) bool {
	return pod.DeletionTimestamp != nil
}
