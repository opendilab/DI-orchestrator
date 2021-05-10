package k8s

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
	"go-sensephoenix.sensetime.com/nervex-operator/server/dynamic"
	servertypes "go-sensephoenix.sensetime.com/nervex-operator/server/types"
	nervexutil "go-sensephoenix.sensetime.com/nervex-operator/utils"
)

func GetNerveXJob(dyi dynamic.Informers, namespace, coordinatorName string) (*nervexv1alpha1.NerveXJob, error) {
	// get coordinator
	coorKey := nervexutil.NamespacedName(namespace, coordinatorName)
	coordinator, err := GetPodByKey(dyi, coorKey)
	if err != nil {
		return nil, err
	}

	var ownRefer metav1.OwnerReference
	ownRefers := coordinator.GetOwnerReferences()
	ownByNerveX := false
	for _, ref := range ownRefers {
		if ref.Kind == nervexv1alpha1.KindNerveXJob {
			ownRefer = ref
			ownByNerveX = true
		}
	}
	if !ownByNerveX {
		errMsg := fmt.Sprintf("coordinator %s is not owned by any NerveXJob", coordinatorName)
		return nil, &servertypes.NerveXError{Type: servertypes.ErrorNotFound, Message: errMsg}
	}

	// get NerveXJob
	njKey := nervexutil.NamespacedName(namespace, ownRefer.Name)
	nvxJob, err := GetNerveXJobByKey(dyi, njKey)
	if err != nil {
		return nil, err
	}

	return nvxJob, nil
}

func GetNerveXJobByKey(dyi dynamic.Informers, key string) (*nervexv1alpha1.NerveXJob, error) {
	obj, exists, err := dyi.NJInformer.Informer().GetIndexer().GetByKey(key)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get NerveXJob: %s", err)
		return nil, fmt.Errorf(errMsg)
	}

	if !exists {
		errMsg := fmt.Sprintf("NerveXJob: %s not exists in cache", key)
		return nil, &servertypes.NerveXError{Type: servertypes.ErrorNotFound, Message: errMsg}
	}
	nvxUn := obj.(*unstructured.Unstructured)
	var nvxJob nervexv1alpha1.NerveXJob
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(nvxUn.UnstructuredContent(), &nvxJob)
	if err != nil {
		errMsg := fmt.Sprintf("failed to convert unstructured: %s", nvxUn.UnstructuredContent())
		return nil, fmt.Errorf(errMsg)
	}

	return &nvxJob, nil
}

func CreateCollectorsAndLearnersForNerveXJob(
	kubeClient *kubernetes.Clientset,
	njreq *servertypes.NerveXJobRequest,
	job *nervexv1alpha1.NerveXJob) ([]string, []string, error) {

	// build owner reference
	ownRefer := metav1.OwnerReference{
		APIVersion: job.APIVersion,
		Kind:       job.Kind,
		Name:       job.Name,
		UID:        job.GetUID(),
		Controller: func(c bool) *bool { return &c }(true),
	}

	// create collectors
	collectorTemplate := job.Spec.Collector.Template
	collectors, err := CreateReplicas(kubeClient, &collectorTemplate, ownRefer, njreq.Collectors, njreq.Namespace, nervexutil.CollectorName)

	if err != nil {
		return collectors, nil, err
	}

	// create learners
	learnerTemplate := job.Spec.Learner.Template
	learners, err := CreateReplicas(kubeClient, &learnerTemplate, ownRefer, njreq.Collectors, njreq.Namespace, nervexutil.LearnerName)

	if err != nil {
		return collectors, learners, err
	}

	return collectors, learners, nil
}

func CreateReplicas(
	kubeClient *kubernetes.Clientset,
	template *corev1.PodTemplateSpec,
	ownRefer metav1.OwnerReference,
	resources servertypes.ResourceQuantity,
	namespace string,
	replicaType string) ([]string, error) {

	var containerName, portName string
	var defaultPort int32
	switch replicaType {
	case nervexutil.CollectorName:
		containerName = nervexutil.DefaultCollectorContainerName
		portName = nervexutil.DefaultCollectorPortName
		defaultPort = nervexutil.DefaultCollectorPort
	case nervexutil.LearnerName:
		containerName = nervexutil.DefaultLearnerContainerName
		portName = nervexutil.DefaultLearnerPortName
		defaultPort = nervexutil.DefaultLearnerPort
	default:

	}
	results := []string{}
	// create pods and services
	for i := 0; i < resources.Replicas; i++ {
		// build pod
		pod, port, err := nervexutil.BuildPodFromTemplate(template.DeepCopy(), ownRefer, namespace, replicaType, containerName, portName, defaultPort)
		if err != nil {
			return results, err
		}
		// set pod resources
		SetPodResources(pod, resources, containerName)

		// build service
		svc := nervexutil.BuildService(pod.GetLabels(), port, portName)
		svc.SetOwnerReferences([]metav1.OwnerReference{ownRefer})
		svc.Name = pod.Name

		// create pod
		if err := CreatePodAndService(kubeClient, namespace, pod, svc); err != nil {
			return results, err
		}

		result := nervexutil.ConcatURL(svc.Name, namespace, port)
		results = append(results, result)
	}

	return results, nil
}

func DeleteReplicas(kubeClient *kubernetes.Clientset, pods []*corev1.Pod, namespace string, replicas int, replicaType string) ([]string, error) {
	var containerName, portName string
	var defaultPort int32

	switch replicaType {
	case nervexutil.CollectorName:
		containerName = nervexutil.DefaultCollectorContainerName
		portName = nervexutil.DefaultCollectorPortName
		defaultPort = nervexutil.DefaultCollectorPort
	case nervexutil.LearnerName:
		containerName = nervexutil.DefaultLearnerContainerName
		portName = nervexutil.DefaultLearnerPortName
		defaultPort = nervexutil.DefaultLearnerPort
	default:

	}

	results := []string{}
	for _, pod := range pods {
		// break if enough
		if len(results) >= replicas {
			break
		}

		// delete pods and services
		if err := DeletePodAndService(kubeClient, namespace, pod.Name); err != nil {
			return results, err
		}

		result := nervexutil.GetPodAccessURL(pod, namespace, containerName, portName, defaultPort)
		results = append(results, result)
	}

	return results, nil
}

func RecreateReplicas(kubeClient *kubernetes.Clientset, pods []*corev1.Pod, namespace, replicaType string) ([]string, error) {
	var containerName, portName string
	var defaultPort int32
	switch replicaType {
	case nervexutil.CollectorName:
		containerName = nervexutil.DefaultCollectorContainerName
		portName = nervexutil.DefaultCollectorPortName
		defaultPort = nervexutil.DefaultCollectorPort
	case nervexutil.LearnerName:
		containerName = nervexutil.DefaultLearnerContainerName
		portName = nervexutil.DefaultLearnerPortName
		defaultPort = nervexutil.DefaultLearnerPort
	default:

	}

	// delete pods and services
	for _, pod := range pods {
		if err := DeletePodAndService(kubeClient, namespace, pod.Name); err != nil {
			return nil, err
		}
	}

	// create pods and services
	var results []string
	for _, oldPod := range pods {
		var pod *corev1.Pod = &corev1.Pod{}
		parts := strings.Split(oldPod.Name, "-")
		generateName := strings.Join(parts[:len(parts)-1], "-")
		name := nervexutil.GenerateName(generateName)

		pod.SetName(name)
		pod.SetOwnerReferences(oldPod.GetOwnerReferences())
		pod.Spec = oldPod.DeepCopy().Spec

		labels := oldPod.GetLabels()
		labels[nervexutil.PodNameLabel] = name
		nervexutil.AddLabelsToPod(pod, labels)

		// build service
		port, ok := nervexutil.GetPortFromPod(pod, containerName, portName)
		if !ok {
			port = defaultPort
		}
		svc := nervexutil.BuildService(pod.GetLabels(), port, portName)
		svc.SetOwnerReferences(pod.GetOwnerReferences())
		svc.Name = pod.Name

		if err := CreatePodAndService(kubeClient, namespace, pod, svc); err != nil {
			return results, err
		}

		result := nervexutil.ConcatURL(svc.Name, namespace, port)
		results = append(results, result)
	}

	return results, nil
}
