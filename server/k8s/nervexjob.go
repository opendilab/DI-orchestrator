package k8s

import (
	"fmt"

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
	collectors, err := CreatePodsAndServices(kubeClient, &collectorTemplate, ownRefer, njreq,
		nervexutil.CollectorName, nervexutil.DefaultCollectorContainerName, nervexutil.DefaultCollectorPortName, nervexutil.DefaultCollectorPort)

	if err != nil {
		return collectors, nil, err
	}

	// create learners
	learnerTemplate := job.Spec.Learner.Template
	learners, err := CreatePodsAndServices(kubeClient, &learnerTemplate, ownRefer, njreq,
		nervexutil.LearnerName, nervexutil.DefaultLearnerContainerName, nervexutil.DefaultLearnerPortName, nervexutil.DefaultLearnerPort)

	if err != nil {
		return collectors, learners, err
	}

	return collectors, learners, nil
}
