package http

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
	commontypes "opendilab.org/di-orchestrator/pkg/common/types"
)

var (
	statusUpdateRetries        = 3
	statusUpdatedPauseDuration = 50 * time.Millisecond
)

func (s *DIServer) getDIJob(namespace, name string) (*div1alpha2.DIJob, error) {
	diUn, err := s.DIClient.Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	var job div1alpha2.DIJob
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(diUn.UnstructuredContent(), &job)
	if err != nil {
		errMsg := fmt.Sprintf("failed to convert unstructured: %s", diUn.UnstructuredContent())
		return nil, fmt.Errorf(errMsg)
	}
	return &job, nil
}

func (s *DIServer) getCachedDIJobByKey(key string) (*div1alpha2.DIJob, error) {
	obj, exists, err := s.dyi.DIInformer.Informer().GetIndexer().GetByKey(key)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get DIJob: %s", err)
		return nil, fmt.Errorf(errMsg)
	}
	if !exists {
		errMsg := fmt.Sprintf("DIJob: %s not exists in cache", key)
		return nil, &commontypes.DIError{Type: commontypes.ErrorNotFound, Message: errMsg}
	}

	diUn := obj.(*unstructured.Unstructured)
	var diJob div1alpha2.DIJob
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(diUn.UnstructuredContent(), &diJob)
	if err != nil {
		errMsg := fmt.Sprintf("failed to convert unstructured: %s", diUn.UnstructuredContent())
		return nil, fmt.Errorf(errMsg)
	}
	return &diJob, nil
}

func (s *DIServer) needMultiDDPLearnerPod(resource commontypes.ResourceQuantity) (bool, error) {
	if err := s.SyncNodes(); err != nil {
		return false, err
	}
	gpusMajority := s.gpuAllocator.NumGPUsOfMajorityNodeType()
	if gpusMajority <= 0 {
		return false, nil
	}

	if int(resource.GPU.Value()) > gpusMajority {
		return true, nil
	}
	return false, nil
}

func (s *DIServer) updateDIJobStatusInCluster(job *div1alpha2.DIJob) error {
	var err error
	for i := 0; i < statusUpdateRetries; i++ {
		newJob := &div1alpha2.DIJob{}
		job, err := s.getDIJob(job.Namespace, job.Name)
		if err != nil {
			break
		}
		newJob.Status = job.Status
		jobMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(newJob)
		if err != nil {
			break
		}
		jobUn := &unstructured.Unstructured{Object: jobMap}
		if _, err := s.DIClient.Namespace(job.Namespace).Update(context.Background(), jobUn, metav1.UpdateOptions{}); err == nil {
			time.Sleep(statusUpdatedPauseDuration)
			break
		}
	}
	return err
}
