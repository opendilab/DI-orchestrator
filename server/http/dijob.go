package http

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	div1alpha1 "opendilab.org/di-orchestrator/api/v1alpha1"
	servertypes "opendilab.org/di-orchestrator/server/types"
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
		return nil, &servertypes.DIError{Type: servertypes.ErrorNotFound, Message: errMsg}
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
			return nil, &servertypes.DIError{Type: servertypes.ErrorNotFound, Message: errMsg}
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
		return nil, &servertypes.DIError{Type: servertypes.ErrorNotFound, Message: errMsg}
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
		return nil, &servertypes.DIError{Type: servertypes.ErrorNotFound, Message: errMsg}
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
	njreq *servertypes.DIJobRequest,
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
	collectors, err := s.createReplicas(&collectorTemplate, volumes, ownRefer, njreq.Collectors, njreq.Namespace, diutil.CollectorName, nil)
	if err != nil {
		return collectors, nil, err
	}

	// create learners
	learnerTemplate := job.Spec.Learner.Template
	learners, err := s.createReplicas(&learnerTemplate, volumes, ownRefer, njreq.Learners, njreq.Namespace, diutil.LearnerName, agtemplate)
	if err != nil {
		return collectors, learners, err
	}

	return collectors, learners, nil
}

func (s *DIServer) createReplicas(
	template *corev1.PodTemplateSpec,
	volumes []corev1.Volume,
	ownRefer metav1.OwnerReference,
	resources servertypes.ResourceQuantity,
	namespace string,
	replicaType string,
	agtemplate *corev1.PodTemplateSpec) ([]string, error) {

	var defaultPort int32
	switch replicaType {
	case diutil.CollectorName:
		defaultPort = diutil.DefaultCollectorPort
	case diutil.LearnerName:
		defaultPort = diutil.DefaultLearnerPort
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
		if replicaType == diutil.LearnerName && needMultiDDPLearnerPod {
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

			result := diutil.ConcatURL(aggsvc.Name, namespace, port)
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
						Port: int32(diutil.DefaultMasterPort),
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

			if replicaType == diutil.LearnerName && needAggregator(resources) {
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
			result := diutil.ConcatURL(svcName, namespace, port)
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

func (s *DIServer) needMultiDDPLearnerPod(resource servertypes.ResourceQuantity) (bool, error) {
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

func (s *DIServer) buildAggregatorPod(
	template *corev1.PodTemplateSpec, ownRefer metav1.OwnerReference,
	jobName, namespace string) (*corev1.Pod, *corev1.Service, int32, error) {
	// create aggregator
	aresource := servertypes.ResourceQuantity{}
	pod, svc, port, err := s.buildPodAndService(template, ownRefer, jobName, namespace,
		diutil.AggregatorName, diutil.DefaultAggregatorPort, aresource, nil)
	if err != nil {
		return nil, nil, -1, err
	}

	// add env
	envs := make(map[string]string)
	envs[diutil.PodNamespaceEnv] = pod.Namespace
	envs[diutil.PodNameEnv] = pod.Name
	envs[diutil.ServerURLEnv] = diutil.DefaultServerURL
	diutil.SetPodEnv(pod, envs)

	return pod, svc, port, nil
}

func (s *DIServer) buildDDPLearnerPodAndService(template *corev1.PodTemplateSpec,
	ownRefer metav1.OwnerReference,
	aggOwnRefer metav1.OwnerReference,
	jobName, namespace, replicaType string,
	defaultPort int32, resources servertypes.ResourceQuantity, volumes []corev1.Volume) (*corev1.Pod, *corev1.Service, int32, error) {
	// make sure aggregator is the controller of ddp learners
	aggOwnRefer.Controller = func(c bool) *bool { return &c }(true)
	ownRefer.Controller = func(c bool) *bool { return &c }(false)

	pod, svc, port, err := s.buildPodAndService(template.DeepCopy(), aggOwnRefer, jobName,
		namespace, diutil.DDPLearnerName, defaultPort, resources, volumes)
	if err != nil {
		return nil, nil, -1, err
	}

	// set owner reference of DIJob to aggregator
	pod.OwnerReferences = append(pod.OwnerReferences, ownRefer)
	svc.OwnerReferences = append(svc.OwnerReferences, ownRefer)

	// create port for all the GPU processes
	for j := 1; j < int(resources.GPU.Value()); j++ {
		pname := fmt.Sprintf("%s-%d", diutil.DDPLearnerPortPrefix, j)
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
			if pod.Spec.Containers[i].Name != diutil.DefaultContainerName {
				continue
			}
			pod.Spec.Containers[i].Ports = append(pod.Spec.Containers[i].Ports, lcport)
		}
	}
	return pod, svc, port, nil
}

func addDDPEnvsToDDPLearner(pod *corev1.Pod, masterAddr string, worldSize, localWorldSize, startRank int) {
	envs := make(map[string]string)
	envs[diutil.WorldSize] = strconv.Itoa(worldSize)
	envs[diutil.LocalWorldSize] = strconv.Itoa(localWorldSize)
	envs[diutil.MasterAddr] = masterAddr
	envs[diutil.MasterPort] = strconv.Itoa(diutil.DefaultMasterPort)
	envs[diutil.StartRank] = strconv.Itoa(startRank)
	diutil.SetPodEnv(pod, envs)
}

func (s *DIServer) deleteSpecifiedReplicas(pods []*corev1.Pod, namespace string, replicas int, replicaType string) ([]string, error) {
	var defaultPort int32

	switch replicaType {
	case diutil.CollectorName:
		defaultPort = diutil.DefaultCollectorPort
	case diutil.LearnerName:
		defaultPort = diutil.DefaultLearnerPort
	case diutil.AggregatorName:
		defaultPort = diutil.DefaultAggregatorPort
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

		result := diutil.GetPodAccessURL(pod, defaultPort)
		results = append(results, result)
	}

	return results, nil
}

func (s *DIServer) recreateReplicas(pods []*corev1.Pod, services []*corev1.Service, namespace string) ([]string, error) {
	var results []string
	for i := range pods {
		oldPod := pods[i]
		replicaType := oldPod.Labels[diutil.ReplicaTypeLabel]
		var defaultPort int32
		var needDDPLearner bool = false
		switch replicaType {
		case diutil.CollectorName:
			defaultPort = diutil.DefaultCollectorPort
		case diutil.LearnerName:
			defaultPort = diutil.DefaultLearnerPort
		case diutil.DDPLearnerName:
			defaultPort = diutil.DefaultLearnerPort
		case diutil.AggregatorName:
			defaultPort = diutil.DefaultAggregatorPort
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
			if port.Name == diutil.DefaultPortName {
				portvalue = port.Port
			}
		}
		result := diutil.ConcatURL(svc.Name, namespace, portvalue)
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

func (s *DIServer) rebuildDDPLearners(namespace, aggregatorName string) ([]*corev1.Pod, []*corev1.Service, error) {
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
			if pod.Spec.Containers[j].Name != diutil.DefaultContainerName {
				continue
			}
			for k := range pod.Spec.Containers[i].Env {
				if pod.Spec.Containers[i].Env[k].Name == diutil.MasterAddr {
					pod.Spec.Containers[i].Env[k].Value = masterAddr
				}
			}
		}

		// delete old pod and service
		s.deletePodAndService(namespace, oldPod.Name)
	}
	return pods, svcs, nil
}

func (s *DIServer) getDDPLearners(namespace, aggregatorName string) ([]*corev1.Pod, []*corev1.Service, error) {
	log := s.Log.WithName("DIServer")

	// get ownReference of the request coordinator
	diJob, err := s.getDIJob(namespace, aggregatorName)
	if err != nil {
		log.Error(err, "failed to get owner reference")
		return nil, nil, err
	}

	// list pods that belong to the DIJob
	labelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: diutil.GenLabels(diJob.Name),
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
