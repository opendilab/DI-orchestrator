package http

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
	servertypes "go-sensephoenix.sensetime.com/nervex-operator/server/types"
	nervexutil "go-sensephoenix.sensetime.com/nervex-operator/utils"
)

func (s *NerveXServer) getNerveXJob(namespace, modulePodName string) (*nervexv1alpha1.NerveXJob, error) {
	// get module
	podKey := nervexutil.NamespacedName(namespace, modulePodName)
	modulePod, err := s.getPodByKey(podKey)
	if err != nil {
		return nil, err
	}

	var ownRefer metav1.OwnerReference
	ownRefers := modulePod.GetOwnerReferences()
	ownByNerveX := false
	for _, ref := range ownRefers {
		if ref.Kind == nervexv1alpha1.KindNerveXJob {
			ownRefer = ref
			ownByNerveX = true
		}
	}
	if !ownByNerveX {
		errMsg := fmt.Sprintf("module %s is not owned by any NerveXJob", modulePodName)
		return nil, &servertypes.NerveXError{Type: servertypes.ErrorNotFound, Message: errMsg}
	}

	// get NerveXJob
	njKey := nervexutil.NamespacedName(namespace, ownRefer.Name)
	nvxJob, err := s.getNerveXJobByKey(njKey)
	if err != nil {
		return nil, err
	}

	// check if the coordinator is owned by stale NerveXJob
	for _, ref := range ownRefers {
		if ref.Kind == nervexv1alpha1.KindNerveXJob && ref.UID != nvxJob.UID {
			errMsg := fmt.Sprintf("coordinator %s is owned by stale NerveXJob", modulePodName)
			return nil, &servertypes.NerveXError{Type: servertypes.ErrorNotFound, Message: errMsg}
		}
	}

	return nvxJob, nil
}

func (s *NerveXServer) getNerveXJobByKey(key string) (*nervexv1alpha1.NerveXJob, error) {
	obj, exists, err := s.dyi.NJInformer.Informer().GetIndexer().GetByKey(key)
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

func (s *NerveXServer) getAGConfigByKey(key string) (*nervexv1alpha1.AggregatorConfig, error) {
	obj, exists, err := s.dyi.AGInformer.Informer().GetIndexer().GetByKey(key)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get AGConfig: %s", err)
		return nil, fmt.Errorf(errMsg)
	}

	if !exists {
		errMsg := fmt.Sprintf("AGConfig: %s not exists in cache", key)
		return nil, &servertypes.NerveXError{Type: servertypes.ErrorNotFound, Message: errMsg}
	}
	agUn := obj.(*unstructured.Unstructured)
	var agconfig nervexv1alpha1.AggregatorConfig
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(agUn.UnstructuredContent(), &agconfig)
	if err != nil {
		errMsg := fmt.Sprintf("failed to convert unstructured: %s", agUn.UnstructuredContent())
		return nil, fmt.Errorf(errMsg)
	}

	return &agconfig, nil
}

func (s *NerveXServer) createCollectorsAndLearnersForNerveXJob(
	njreq *servertypes.NerveXJobRequest,
	job *nervexv1alpha1.NerveXJob) ([]string, []string, error) {

	// get agconfig to fetch aggregator definition
	agKey := s.AGConfig
	agconfig, err := s.getAGConfigByKey(agKey)
	if err != nil {
		return nil, nil, err
	}

	// build aggregator pod
	agtemplate := agconfig.Spec.Aggregator.Template.DeepCopy()

	// build owner reference
	ownRefer := metav1.OwnerReference{
		APIVersion: job.APIVersion,
		Kind:       job.Kind,
		Name:       job.Name,
		UID:        job.GetUID(),
		Controller: func(c bool) *bool { return &c }(true),
	}

	volumes := job.Spec.Volumes
	// create collectors
	collectorTemplate := job.Spec.Collector.Template
	collectors, err := s.createReplicas(&collectorTemplate, volumes, ownRefer, njreq.Collectors, njreq.Namespace, nervexutil.CollectorName, nil)
	if err != nil {
		return collectors, nil, err
	}

	// create learners
	learnerTemplate := job.Spec.Learner.Template
	learners, err := s.createReplicas(&learnerTemplate, volumes, ownRefer, njreq.Learners, njreq.Namespace, nervexutil.LearnerName, agtemplate)
	if err != nil {
		return collectors, learners, err
	}

	return collectors, learners, nil
}

func (s *NerveXServer) createReplicas(
	template *corev1.PodTemplateSpec,
	volumes []corev1.Volume,
	ownRefer metav1.OwnerReference,
	resources servertypes.ResourceQuantity,
	namespace string,
	replicaType string,
	agtemplate *corev1.PodTemplateSpec) ([]string, error) {

	var defaultPort int32
	switch replicaType {
	case nervexutil.CollectorName:
		defaultPort = nervexutil.DefaultCollectorPort
	case nervexutil.LearnerName:
		defaultPort = nervexutil.DefaultLearnerPort
	default:

	}
	results := []string{}
	// create pods and services
	for i := 0; i < resources.Replicas; i++ {
		var pod *corev1.Pod
		var svc *corev1.Service
		var port int32
		var podList []*corev1.Pod
		var svcList []*corev1.Service

		// check if we need to create multiple pods for learner
		needMultiDDPLearnerPod, err := s.needMultiDDPLearnerPod(resources)
		if err != nil {
			return results, err
		}

		// if we need to create multiple ddp learner pods, we also need to create aggregator pod
		if replicaType == nervexutil.LearnerName && needMultiDDPLearnerPod {
			jobName := ownRefer.Name
			// create aggregator
			agg, aggsvc, port, err := s.buildAggregatorPod(agtemplate, ownRefer, jobName, namespace)
			if err != nil {
				return results, err
			}

			newagg, err := s.createPodAndService(namespace, agg, aggsvc)
			if err != nil {
				return results, err
			}
			aggOwnRefer := metav1.OwnerReference{
				APIVersion: corev1.SchemeGroupVersion.Version,
				Kind:       "Pod",
				Name:       newagg.Name,
				UID:        newagg.UID,
				Controller: func(c bool) *bool { return &c }(false),
			}

			result := nervexutil.ConcatURL(aggsvc.Name, namespace, port)
			results = append(results, result)

			// allocate gpus
			worldSize := int(resources.GPU.Value())
			gpuSlice := s.gpuAllocator.Allocate(worldSize)
			startRank := 0
			for j, gpus := range gpuSlice {
				replicaResource := resources.DeepCopy()
				replicaResource.GPU = resource.MustParse(fmt.Sprintf("%d", gpus))

				// build ddp learner pod
				pod, svc, _, err = s.buildDDPLearnerPodAndService(template, ownRefer, aggOwnRefer,
					jobName, namespace, replicaType, defaultPort, *replicaResource, volumes)
				if err != nil {
					return results, err
				}

				// append pod and svc to list
				podList = append(podList, pod)
				svcList = append(svcList, svc)

				// add ddp envs to ddp learner
				masterAddr := svcList[0].Name
				if j == 0 {
					masterAddr = "localhost"
					// set access port for ddp master port
					mport := corev1.ServicePort{
						Name: "master-port",
						Port: int32(nervexutil.DefaultMasterPort),
					}
					svcList[0].Spec.Ports = append(svcList[0].Spec.Ports, mport)
				}
				addDDPEnvsToDDPLearner(pod, masterAddr, worldSize, gpus, startRank)
				startRank += gpus
			}

		} else {
			var svcName string
			jobName := ownRefer.Name

			// build collector/learner pod
			pod, svc, port, err = s.buildPodAndService(template.DeepCopy(), ownRefer, jobName,
				namespace, replicaType, defaultPort, resources, volumes)
			if err != nil {
				return results, err
			}
			svcName = svc.Name

			if replicaType == nervexutil.LearnerName && needAggregator(resources) {
				// create aggregator
				agg, aggsvc, aggport, err := s.buildAggregatorPod(agtemplate, ownRefer, jobName, namespace)
				if err != nil {
					return results, err
				}

				// set logic learner access svc and port
				port = aggport
				svcName = aggsvc.Name

				newagg, err := s.createPodAndService(namespace, agg, aggsvc)
				if err != nil {
					return results, err
				}

				aggOwnRefer := metav1.OwnerReference{
					APIVersion: corev1.SchemeGroupVersion.Version,
					Kind:       "Pod",
					Name:       newagg.Name,
					UID:        newagg.UID,
					Controller: func(c bool) *bool { return &c }(false),
				}

				// build ddp learner pod
				pod, svc, _, err = s.buildDDPLearnerPodAndService(template, ownRefer, aggOwnRefer,
					jobName, namespace, replicaType, defaultPort, resources, volumes)
				if err != nil {
					return results, err
				}

				// add ddp envs to ddp learner pod
				masterAddr := "localhost"
				worldSize := int(resources.GPU.Value())
				addDDPEnvsToDDPLearner(pod, masterAddr, worldSize, worldSize, 0)
			}

			podList = append(podList, pod)
			svcList = append(svcList, svc)
			result := nervexutil.ConcatURL(svcName, namespace, port)
			results = append(results, result)
		}

		for i := 0; i < len(podList); i++ {
			pod := podList[i]
			svc := svcList[i]
			// create pod
			if _, err := s.createPodAndService(namespace, pod, svc); err != nil {
				return results, err
			}
		}
	}
	return results, nil
}

func (s *NerveXServer) needMultiDDPLearnerPod(resource servertypes.ResourceQuantity) (bool, error) {
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

func needAggregator(resource servertypes.ResourceQuantity) bool {
	return resource.GPU.Value() > 1
}

func (s *NerveXServer) buildAggregatorPod(
	template *corev1.PodTemplateSpec, ownRefer metav1.OwnerReference,
	jobName, namespace string) (*corev1.Pod, *corev1.Service, int32, error) {
	// create aggregator
	aresource := servertypes.ResourceQuantity{}
	pod, svc, port, err := s.buildPodAndService(template, ownRefer, jobName, namespace,
		nervexutil.AggregatorName, nervexutil.DefaultAggregatorPort, aresource, nil)
	if err != nil {
		return nil, nil, -1, err
	}

	// add env
	envs := make(map[string]string)
	envs[nervexutil.PodNamespaceEnv] = pod.Namespace
	envs[nervexutil.PodNameEnv] = pod.Name
	envs[nervexutil.ServerURLEnv] = nervexutil.DefaultServerURL
	nervexutil.SetPodEnv(pod, envs)

	return pod, svc, port, nil
}

func (s *NerveXServer) buildDDPLearnerPodAndService(template *corev1.PodTemplateSpec,
	ownRefer metav1.OwnerReference,
	aggOwnRefer metav1.OwnerReference,
	jobName, namespace, replicaType string,
	defaultPort int32, resources servertypes.ResourceQuantity, volumes []corev1.Volume) (*corev1.Pod, *corev1.Service, int32, error) {
	// make sure aggregator is the controller of ddp learners
	aggOwnRefer.Controller = func(c bool) *bool { return &c }(true)
	ownRefer.Controller = func(c bool) *bool { return &c }(false)

	pod, svc, port, err := s.buildPodAndService(template.DeepCopy(), aggOwnRefer, jobName,
		namespace, nervexutil.DDPLearnerName, defaultPort, resources, volumes)
	if err != nil {
		return nil, nil, -1, err
	}

	// set owner reference of NerveXJob to aggregator
	pod.OwnerReferences = append(pod.OwnerReferences, ownRefer)
	svc.OwnerReferences = append(svc.OwnerReferences, ownRefer)

	// create port for all the GPU processes
	for j := 1; j < int(resources.GPU.Value()); j++ {
		pname := fmt.Sprintf("%s-%d", nervexutil.DDPLearnerPortPrefix, j)
		pport := port + int32(j)
		lsport := corev1.ServicePort{
			Name: pname,
			Port: pport,
		}
		svc.Spec.Ports = append(svc.Spec.Ports, lsport)

		lcport := corev1.ContainerPort{
			Name:          pname,
			ContainerPort: pport,
		}
		for i := range pod.Spec.Containers {
			if pod.Spec.Containers[i].Name != nervexutil.DefaultContainerName {
				continue
			}
			pod.Spec.Containers[i].Ports = append(pod.Spec.Containers[i].Ports, lcport)
		}
	}
	return pod, svc, port, nil
}

func addDDPEnvsToDDPLearner(pod *corev1.Pod, masterAddr string, worldSize, localWorldSize, startRank int) {
	envs := make(map[string]string)
	envs[nervexutil.WorldSize] = strconv.Itoa(worldSize)
	envs[nervexutil.LocalWorldSize] = strconv.Itoa(localWorldSize)
	envs[nervexutil.MasterAddr] = masterAddr
	envs[nervexutil.MasterPort] = strconv.Itoa(nervexutil.DefaultMasterPort)
	envs[nervexutil.StartRank] = strconv.Itoa(startRank)
	nervexutil.SetPodEnv(pod, envs)
}

func (s *NerveXServer) deleteSpecifiedReplicas(pods []*corev1.Pod, namespace string, replicas int, replicaType string) ([]string, error) {
	var defaultPort int32

	switch replicaType {
	case nervexutil.CollectorName:
		defaultPort = nervexutil.DefaultCollectorPort
	case nervexutil.LearnerName:
		defaultPort = nervexutil.DefaultLearnerPort
	case nervexutil.AggregatorName:
		defaultPort = nervexutil.DefaultAggregatorPort
	default:

	}

	results := []string{}
	for _, pod := range pods {
		// break if enough
		if len(results) >= replicas {
			break
		}

		// delete pods and services
		if err := s.deletePodAndService(namespace, pod.Name); err != nil {
			return results, err
		}

		result := nervexutil.GetPodAccessURL(pod, defaultPort)
		results = append(results, result)
	}

	return results, nil
}

func (s *NerveXServer) recreateReplicas(pods []*corev1.Pod, services []*corev1.Service, namespace string) ([]string, error) {
	var results []string
	for i := range pods {
		oldPod := pods[i]
		replicaType := oldPod.Labels[nervexutil.ReplicaTypeLabel]
		var defaultPort int32
		var needDDPLearner bool = false
		switch replicaType {
		case nervexutil.CollectorName:
			defaultPort = nervexutil.DefaultCollectorPort
		case nervexutil.LearnerName:
			defaultPort = nervexutil.DefaultLearnerPort
		case nervexutil.DDPLearnerName:
			defaultPort = nervexutil.DefaultLearnerPort
		case nervexutil.AggregatorName:
			defaultPort = nervexutil.DefaultAggregatorPort
			needDDPLearner = true
		default:
			return results, fmt.Errorf("unknown replica type")
		}

		// build new ddp learners
		var ddppods []*corev1.Pod
		var ddpsvcs []*corev1.Service
		var err error
		if needDDPLearner {
			aggregatorName := oldPod.Name
			ddppods, ddpsvcs, err = s.rebuildDDPLearners(namespace, aggregatorName)
			if err != nil {
				return results, err
			}
		}

		// delete pods and services
		if err := s.deletePodAndService(namespace, oldPod.Name); err != nil {
			return results, err
		}

		oldService := services[i]
		pod, svc := rebuildPodAndService(oldPod, oldService)

		if _, err := s.createPodAndService(namespace, pod, svc); err != nil {
			return results, err
		}

		var portvalue int32 = defaultPort
		for _, port := range svc.Spec.Ports {
			if port.Name == nervexutil.DefaultPortName {
				portvalue = port.Port
			}
		}
		result := nervexutil.ConcatURL(svc.Name, namespace, portvalue)
		results = append(results, result)

		if needDDPLearner {
			for j := range ddppods {
				newPod := ddppods[j]
				newSvc := ddpsvcs[j]
				if _, err := s.createPodAndService(namespace, newPod, newSvc); err != nil {
					return results, err
				}
			}
		}
	}

	return results, nil
}

func (s *NerveXServer) rebuildDDPLearners(namespace, aggregatorName string) ([]*corev1.Pod, []*corev1.Service, error) {
	ddppods, ddpsvcs, err := s.getDDPLearners(namespace, aggregatorName)
	if err != nil {
		return nil, nil, err
	}
	if len(ddppods) != len(ddpsvcs) {
		return nil, nil, fmt.Errorf("the number of ddp learner pods and services must be the same")
	}

	var pods []*corev1.Pod
	var svcs []*corev1.Service
	for i := range ddppods {
		oldPod := ddppods[i]
		oldService := ddpsvcs[i]
		pod, svc := rebuildPodAndService(oldPod, oldService)
		pods = append(pods, pod)
		svcs = append(svcs, svc)

		// add env
		masterAddr := svcs[0].Name
		if i == 0 {
			masterAddr = "localhost"
		}
		for j := range pod.Spec.Containers {
			if pod.Spec.Containers[j].Name != nervexutil.DefaultContainerName {
				continue
			}
			for k := range pod.Spec.Containers[i].Env {
				if pod.Spec.Containers[i].Env[k].Name == nervexutil.MasterAddr {
					pod.Spec.Containers[i].Env[k].Value = masterAddr
				}
			}
		}

		// delete old pod and service
		s.deletePodAndService(namespace, oldPod.Name)
	}
	return pods, svcs, nil
}

func (s *NerveXServer) getDDPLearners(namespace, aggregatorName string) ([]*corev1.Pod, []*corev1.Service, error) {
	log := s.Log.WithName("NerveXServer")

	// get ownReference of the request coordinator
	nvxJob, err := s.getNerveXJob(namespace, aggregatorName)
	if err != nil {
		log.Error(err, "failed to get owner reference")
		return nil, nil, err
	}

	// list pods that belong to the NerveXJob
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: nervexutil.GenLabels(nvxJob.Name),
	})
	if err != nil {
		return nil, nil, err
	}
	_, _, _, _, DDPLearners, err := s.listReplicaPodsWithSelector(namespace, labelSelector)
	if err != nil {
		log.Error(err, "failed to list collectors and learners")
		return nil, nil, err
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

	_, _, _, _, DDPLearnerServices, err := s.listReplicaServicesWithSelector(namespace, labelSelector)
	if err != nil {
		log.Error(err, "failed to list collectors and learners")
		return nil, nil, err
	}

	var ddpsvcs []*corev1.Service
	for _, pod := range DDPLearnerServices {
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
		ddpsvcs = append(ddpsvcs, pod)
	}

	return ddppods, ddpsvcs, nil
}
