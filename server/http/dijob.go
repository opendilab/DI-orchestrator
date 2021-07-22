package http

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	div1alpha1 "opendilab.org/di-orchestrator/api/v1alpha1"
	dicommon "opendilab.org/di-orchestrator/common"
	commontypes "opendilab.org/di-orchestrator/common/types"
	diutil "opendilab.org/di-orchestrator/utils"
)

func (s *DIServer) getDIJob(namespace, modulePodName string) (*div1alpha1.DIJob, error) {
	// get module
	podKey := diutil.NamespacedName(namespace, modulePodName)
	modulePod, err := s.getPodByKey(podKey)
	if err != nil {
		return nil, err
	}

	var ownRefer metav1.OwnerReference
	ownRefers := modulePod.GetOwnerReferences()
	ownByDI := false
	for _, ref := range ownRefers {
		if ref.Kind == div1alpha1.KindDIJob {
			ownRefer = ref
			ownByDI = true
		}
	}
	if !ownByDI {
		errMsg := fmt.Sprintf("module %s is not owned by any DIJob", modulePodName)
		return nil, &commontypes.DIError{Type: commontypes.ErrorNotFound, Message: errMsg}
	}

	// get DIJob
	njKey := diutil.NamespacedName(namespace, ownRefer.Name)
	diJob, err := s.getDIJobByKey(njKey)
	if err != nil {
		return nil, err
	}

	// check if the coordinator is owned by stale DIJob
	for _, ref := range ownRefers {
		if ref.Kind == div1alpha1.KindDIJob && ref.UID != diJob.UID {
			errMsg := fmt.Sprintf("coordinator %s is owned by stale DIJob", modulePodName)
			return nil, &commontypes.DIError{Type: commontypes.ErrorNotFound, Message: errMsg}
		}
	}

	return diJob, nil
}

func (s *DIServer) getDIJobByKey(key string) (*div1alpha1.DIJob, error) {
	obj, exists, err := s.dyi.NJInformer.Informer().GetIndexer().GetByKey(key)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get DIJob: %s", err)
		return nil, fmt.Errorf(errMsg)
	}

	if !exists {
		errMsg := fmt.Sprintf("DIJob: %s not exists in cache", key)
		return nil, &commontypes.DIError{Type: commontypes.ErrorNotFound, Message: errMsg}
	}
	diUn := obj.(*unstructured.Unstructured)
	var diJob div1alpha1.DIJob
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(diUn.UnstructuredContent(), &diJob)
	if err != nil {
		errMsg := fmt.Sprintf("failed to convert unstructured: %s", diUn.UnstructuredContent())
		return nil, fmt.Errorf(errMsg)
	}

	return &diJob, nil
}

func (s *DIServer) getAGConfigByKey(key string) (*div1alpha1.AggregatorConfig, error) {
	obj, exists, err := s.dyi.AGInformer.Informer().GetIndexer().GetByKey(key)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get AGConfig: %s", err)
		return nil, fmt.Errorf(errMsg)
	}

	if !exists {
		errMsg := fmt.Sprintf("AGConfig: %s not exists in cache", key)
		return nil, &commontypes.DIError{Type: commontypes.ErrorNotFound, Message: errMsg}
	}
	agUn := obj.(*unstructured.Unstructured)
	var agconfig div1alpha1.AggregatorConfig
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(agUn.UnstructuredContent(), &agconfig)
	if err != nil {
		errMsg := fmt.Sprintf("failed to convert unstructured: %s", agUn.UnstructuredContent())
		return nil, fmt.Errorf(errMsg)
	}

	return &agconfig, nil
}

func (s *DIServer) createCollectorsAndLearnersForDIJob(
	njreq *commontypes.DIJobRequest,
	job *div1alpha1.DIJob) ([]string, []string, error) {

	// get agconfig to fetch aggregator definition
	agKey := s.AGConfig
	agconfig, err := s.getAGConfigByKey(agKey)
	if err != nil {
		return nil, nil, err
	}

	// build aggregator pod
	agtemplate := agconfig.Spec.Aggregator.Template.DeepCopy()

	// build owner reference
	ownRefer := diutil.NewOwnerReference(job.APIVersion, job.Kind, job.Name, job.UID, true)

	volumes := job.Spec.Volumes
	// create collectors
	collectorTemplate := job.Spec.Collector.Template
	collectors, err := s.createReplicas(&collectorTemplate, volumes, ownRefer, njreq.Collectors, njreq.Namespace, dicommon.CollectorName, nil)
	if err != nil {
		return collectors, nil, err
	}

	// create learners
	learnerTemplate := job.Spec.Learner.Template
	learners, err := s.createReplicas(&learnerTemplate, volumes, ownRefer, njreq.Learners, njreq.Namespace, dicommon.LearnerName, agtemplate)
	if err != nil {
		return collectors, learners, err
	}

	return collectors, learners, nil
}

func (s *DIServer) createReplicas(
	template *corev1.PodTemplateSpec,
	volumes []corev1.Volume,
	ownRefer metav1.OwnerReference,
	resources commontypes.ResourceQuantity,
	namespace string,
	replicaType string,
	agtemplate *corev1.PodTemplateSpec) ([]string, error) {

	results := []string{}
	// create pods and services
	for i := 0; i < resources.Replicas; i++ {
		if replicaType == dicommon.LearnerName && needAggregator(resources) {
			jobName := ownRefer.Name
			// create aggregator and ddp learners
			result, err := s.createAggregatorAndDDPLearners(template.DeepCopy(), agtemplate, ownRefer,
				jobName, namespace, replicaType, resources, volumes)
			if err != nil {
				return results, err
			}
			results = append(results, result)
		} else {
			jobName := ownRefer.Name

			// build collector pod
			pod, _, port, err := diutil.BuildPodAndService(template.DeepCopy(), ownRefer, jobName,
				namespace, replicaType, volumes)
			if err != nil {
				return results, err
			}
			// set pod resources
			diutil.SetPodResources(pod, resources)

			if _, err := s.createPod(namespace, pod); err != nil {
				return results, err
			}

			result := diutil.ConcatURL(pod.Name, namespace, port)
			results = append(results, result)
		}
	}
	return results, nil
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

func needAggregator(resource commontypes.ResourceQuantity) bool {
	return resource.GPU.Value() > 1
}

func (s *DIServer) createAggregatorAndDDPLearners(template, agtemplate *corev1.PodTemplateSpec,
	ownRefer metav1.OwnerReference,
	jobName, namespace, replicaType string,
	resources commontypes.ResourceQuantity, volumes []corev1.Volume,
) (string, error) {
	// create aggregator
	agg, _, port, err := diutil.BuildPodAndService(agtemplate, ownRefer, jobName, namespace,
		dicommon.AggregatorName, nil)
	if err != nil {
		return "", err
	}

	newagg, err := s.createPod(namespace, agg)
	if err != nil {
		return "", err
	}
	aggOwnRefer := diutil.NewOwnerReference(corev1.SchemeGroupVersion.String(), "Pod", newagg.Name, newagg.UID, false)

	replicaURL := diutil.ConcatURL(agg.Name, namespace, port)

	// check if we need to create multiple pods for learner
	if err := s.createDDPLearnerPods(template, ownRefer, aggOwnRefer,
		jobName, namespace, resources, volumes); err != nil {
		return replicaURL, err
	}

	return replicaURL, nil
}

func (s *DIServer) createDDPLearnerPods(template *corev1.PodTemplateSpec,
	ownRefer, aggOwnRefer metav1.OwnerReference,
	jobName, namespace string,
	resources commontypes.ResourceQuantity, volumes []corev1.Volume,
) error {
	needMultiDDPLearnerPod, err := s.needMultiDDPLearnerPod(resources)
	if err != nil {
		return err
	}

	if needMultiDDPLearnerPod {
		// allocate gpus
		worldSize := int(resources.GPU.Value())
		gpuSlice := s.gpuAllocator.Allocate(worldSize)
		startRank := 0
		var masterPod *corev1.Pod
		for j, gpus := range gpuSlice {
			replicaResource := resources.DeepCopy()
			replicaResource.GPU = resource.MustParse(fmt.Sprintf("%d", gpus))

			// build ddp learner pod
			pod, _, err := buildDDPLearnerPodAndService(template, ownRefer, aggOwnRefer,
				jobName, namespace, *replicaResource, volumes)
			if err != nil {
				return err
			}

			// add ddp envs to ddp learner
			var masterAddr string
			if j == 0 {
				masterPod = pod
				masterAddr = "localhost"
				// set access port for ddp master
				mport := corev1.ContainerPort{
					Name:          "master-port",
					ContainerPort: dicommon.DefaultMasterPort,
				}
				diutil.AddPortToPod(pod, mport)
				// set labels for ddp master
				labels := map[string]string{dicommon.DDPLearnerTypeLabel: dicommon.DDPLearnerTypeMaster}
				diutil.AddLabelsToPod(pod, labels)
			} else {
				masterAddr = masterPod.Name
				// set labels for ddp worker
				labels := map[string]string{dicommon.DDPLearnerTypeLabel: dicommon.DDPLearnerTypeWorker}
				diutil.AddLabelsToPod(pod, labels)
			}
			addDDPEnvsToDDPLearner(pod, masterAddr, worldSize, gpus, startRank)
			startRank += gpus

			if _, err := s.createPod(namespace, pod); err != nil {
				return err
			}
		}
	} else {
		// build ddp learner pod
		pod, _, err := buildDDPLearnerPodAndService(template, ownRefer, aggOwnRefer,
			jobName, namespace, resources, volumes)
		if err != nil {
			return err
		}

		// add ddp envs to ddp learner pod
		masterAddr := "localhost"
		worldSize := int(resources.GPU.Value())
		addDDPEnvsToDDPLearner(pod, masterAddr, worldSize, worldSize, 0)

		if _, err := s.createPod(namespace, pod); err != nil {
			return err
		}
	}
	return nil
}

func buildDDPLearnerPodAndService(template *corev1.PodTemplateSpec,
	ownRefer, aggOwnRefer metav1.OwnerReference,
	jobName, namespace string,
	resources commontypes.ResourceQuantity, volumes []corev1.Volume) (*corev1.Pod, int32, error) {
	pod, _, port, err := diutil.BuildPodAndService(template.DeepCopy(), ownRefer, jobName,
		namespace, dicommon.DDPLearnerName, volumes)
	if err != nil {
		return nil, -1, err
	}
	diutil.SetPodResources(pod, resources)

	// set owner reference of aggregator to ddp learner
	pod.OwnerReferences = append(pod.OwnerReferences, aggOwnRefer)

	// create port for all the GPU processes
	diutil.AddGPUPortsToPod(pod, int(resources.GPU.Value()), port)
	return pod, port, nil
}

func addDDPEnvsToDDPLearner(pod *corev1.Pod, masterAddr string, worldSize, localWorldSize, startRank int) {
	envs := make(map[string]string)
	envs[dicommon.WorldSize] = strconv.Itoa(worldSize)
	envs[dicommon.LocalWorldSize] = strconv.Itoa(localWorldSize)
	envs[dicommon.MasterAddr] = masterAddr
	envs[dicommon.MasterPort] = strconv.Itoa(dicommon.DefaultMasterPort)
	envs[dicommon.StartRank] = strconv.Itoa(startRank)
	diutil.AddEnvsToPod(pod, envs)
}

func (s *DIServer) deleteSpecifiedReplicas(pods []*corev1.Pod, namespace string, replicas int, replicaType string) ([]string, error) {
	results := []string{}
	for _, pod := range pods {
		// break if enough
		if len(results) >= replicas {
			break
		}

		// delete pods and services
		if err := s.deletePod(namespace, pod.Name); err != nil {
			return results, err
		}

		var defaultPort int32 = diutil.GetReplicaDefaultPort(replicaType)
		result := diutil.GetPodAccessURL(pod, defaultPort)
		results = append(results, result)
	}

	return results, nil
}

func (s *DIServer) recreateReplicas(pods []*corev1.Pod, namespace string) ([]string, error) {
	var results []string
	for i := range pods {
		oldPod := pods[i]
		replicaType := oldPod.Labels[dicommon.ReplicaTypeLabel]

		// get ownReference of the request aggregator
		job, err := s.getDIJob(namespace, oldPod.Name)
		if err != nil {
			return results, err
		}

		// delete pods and services
		if err := s.deletePod(namespace, oldPod.Name); err != nil {
			return results, err
		}

		pod := diutil.RebuildPod(oldPod)
		port, ok := diutil.GetDefaultPortFromPod(pod)
		if !ok {
			port = diutil.GetReplicaDefaultPort(replicaType)
		}

		newpod, err := s.createPod(namespace, pod)
		if err != nil {
			return results, err
		}

		result := diutil.ConcatURL(pod.Name, namespace, port)
		results = append(results, result)

		// build new ddp learners
		needDDPLearner := replicaType == dicommon.AggregatorName
		if needDDPLearner {
			aggregatorName := oldPod.Name
			// get ddp learners of the request aggregator
			ddppods, err := s.getDDPLearners(namespace, job.Name, aggregatorName)
			if err != nil {
				return results, err
			}
			// delete the ddp learners
			err = s.deletePods(ddppods)
			if err != nil {
				return results, err
			}

			// recreate ddp learners
			template := job.Spec.Learner.Template
			ownRefer := diutil.NewOwnerReference(job.APIVersion, job.Kind, job.Name, job.UID, true)
			aggOwnRefer := diutil.NewOwnerReference(corev1.SchemeGroupVersion.String(), "Pod", newpod.Name, newpod.UID, false)
			var resources commontypes.ResourceQuantity
			if len(ddppods) > 0 {
				ddppod := ddppods[0]
				resources = diutil.GetPodResources(ddppod)
				gpus, ok := diutil.GetEnvFromPod(ddppod, dicommon.WorldSize)
				if !ok {
					gpus = "0"
				}
				resources.GPU = resource.MustParse(gpus)
			}
			err = s.createDDPLearnerPods(template.DeepCopy(), ownRefer, aggOwnRefer, job.Name, namespace, resources, job.Spec.Volumes)
			if err != nil {
				return results, err
			}
		}
	}

	return results, nil
}

func (s *DIServer) getDDPLearners(namespace, jobName, aggregatorName string) ([]*corev1.Pod, error) {
	// list pods that belong to the DIJob
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: diutil.GenLabels(jobName),
	})
	if err != nil {
		return nil, err
	}
	ddppods, err := s.getDDPLearnerPods(namespace, aggregatorName, labelSelector)
	if err != nil {
		return nil, err
	}

	return ddppods, nil
}

func (s *DIServer) getDDPLearnerPods(namespace, aggregatorName string, labelSelector labels.Selector) ([]*corev1.Pod, error) {
	log := s.Log.WithName("getDDPLearnerPods")
	_, _, _, _, DDPLearners, err := s.listReplicaPodsWithSelector(namespace, labelSelector)
	if err != nil {
		log.Error(err, "failed to list collectors and learners")
		return nil, err
	}

	var ddppods []*corev1.Pod
	for _, pod := range DDPLearners {
		owners := pod.GetOwnerReferences()
		owns := false
		for _, owner := range owners {
			if owner.Name == aggregatorName {
				owns = true
				break
			}
		}
		if !owns {
			continue
		}
		ddppods = append(ddppods, pod)
	}
	return ddppods, nil
}
