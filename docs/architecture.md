# DI Operator architecture
DI-engine framework consists of 3 important modules, namely coordinator, collector and learner. In general, a DI-engine training job has only one coordinator, and the number of learners and collectors can vary. The roles of the three modules are:
- Coordinator. Maintain connections with collectors and learners, accept meta-infos requests and posts from collectors and learners, and send tasks to collectors and learners.
- Collector. Request path to RL model stored in storage middleware from coordinator, load the RL model, and then generate data frames according to the RL model's steps from environment. Store the data frames back to the storage middleware, and report meta-infos (the storage path, size, etc.) of the data frames to coordinator.
- Learner: Request data frames storage path from coordinator and load the data frames from storage middleware to start training the RL model. After the training is completed, store the model into the storage middleware, and report model meta-infos (storage path, size, etc.) to coordinator. Because we often need to use distributed mechanism of data parallel training, to avoid confusion, we call the module interacting with coordinator the logic learner, which is the basic unit for coordinator to issue tasks. And the single learner process in the data parallel training is called ddp learner, and multiple ddp learner processes provide data parallel services. One logic learner can correspond to one ddp learner (single-gpu) or multiple ddp learners (multi-gpu). In addition, to provide data parallel training services, an additional aggregator module needs to be introduced. The aggregator is responsible for summarizing the training results of multiple ddp learners and sending them to coordinator. That is, the aggregator and multiple ddp learners form a logic learner, and coordinator will only interact with logic learners.

For the introduction of DI-engine, please refer to [DI-engine developer tutorial](https://opendilab.github.io/DI-engine/tutorial_dev/index.html).

In order to provide running support for DI-engine in Kubernetes (K8s), we designed `DI Orchestrator`. This article will explain how to use DI Orchestrator, how each module of DI-engine is created on K8s and discovers each other, how to start training, etc. The architecture of DI Orchestrator is shown in the figure below:

![](images/di-arch.svg)

There are two main modules that is `di-server` and `di-operator`. 
`DDPL` represents ddp learner, `Lm` represents logic learner, `Cn` represents collector, and `Aggregator+DDPL` constructs a logic learner. In the following pages, we will first introduce how `DI Orchestrator` creates and starts each module of DI-engine after a DI-engine job is submitted to K8s, and then introduces the architecture of `di-server` and `di-operator`.

## Job creation process
Here is a description of the job creation process, illustrating the entire life cycle of a DI-engine job from creation to execution in K8s.
- Edit the AggregatorConfig yaml file to define the aggregator template, which will be used to create aggregators when DIJob is created later. Aggregator can provide data parallel training services.
- Edit the DIJob yaml file to define the template of coordinator, collector and learner, and submit it to K8s.
- After di-operator received the event of DIJob submission, it creates a coordinator, and creates an accessible domain name for the coordinator.
- After the coordinator started, it sends an HTTP request to di-server to create a certain number of collectors and learners according to the coordinator's default configuration.
- After di-server receives the coordinator's creation request, it reads the collector and learner templates from DIJob object, and creates the corresponding number of collectors (Cn in the above figure) and learners (Lm in the above figure), and returns the URLs accessible to the collectors and learners. At the same time, di-server will determine whether to create an aggregator for each learner according to the number of GPUs applied for in each learner. That is, when the number of GPUs requested by the learner is greater than 1, an aggregator is created for the learner, otherwise no aggregator is created.
- Coordinator waits for collectors and learners (an aggregator and its multiple ddp learners are regarded as a logic learner) to connect, and then starts to issue tasks to start training.
- We can manually send a request to di-server to add/delete collectors or learners, and the coordinator will periodically query the number of available collectors and learners and decide to create or disconnect connections to them.
- When the training is completed, di-operator will delete all collectors, learners by default, while coordinator will be reserved for users to view logs and other operations.

## DI Operator
Di-operator is a component responsible for orchestrating DIJob in K8s. It uses K8s [operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) to monitor the status of DIJob objects in K8s cluster through the control loop in [controller pattern](https://kubernetes.io/docs/concepts/architecture/controller/), and to update the status of DIJob when necessary. The status is modified so that the actual status of DIJob is as consistent as possible with our predefined status.

### API definition
According to the characteristics of each module, we have defined two Custom Resources, namely DIJob and AggregatorConfig. The former is used to define the prerequisites for coordinator, collector and learner to start running, including docker images, startup commands, computing and storage resources, environment variables, etc. The latter is used to define the prerequisites for aggregator.

DIJob definition is described as below:
```go
type DIJobSpec struct {
	// Group is a collection of DIJobs
	Group string `json:"group,omitempty"`

	//Priority labels the priority of DIJob
	PriorityClassName PriorityClassName `json:"priorityClassName,omitempty"`

	// CleanPodPolicy defines the policy to clean pods after DIJob completed
	CleanPodPolicy CleanPodPolicy `json:"cleanPodPolicy,omitempty"`

	// Volumes defines the shared volumes for DI-engine components
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	Coordinator CoordinatorSpec `json:"coordinator"`

	Collector CollectorSpec `json:"collector,"`

	Learner LearnerSpec `json:"learner,"`
}
```

AggregatorConfig definition is described as below:
```go
type AggregatorConfigSpec struct {
	Aggregator AggregatorSpec `json:"aggregator,"`
}
```

> **Why should aggregator be defined alone?**
    Aggregator is common module for all RL training jobs using DI-engine framework, so we define the aggregator as a global and shared resource named AggregatorConfig. After RL jobs are submitted, di-server will read the global AggregatorConfig in K8s cluster to create aggregators for these RL jobs. In addition, aggregator is only for most common data parallel training. You need to define a new Custom Resource if other parallel training methods are used.
### Status definition
After DIJob is submitted, di-operator takes over the management of the life cycle of the DIJob. In order to facilitate the user to have a better view of the DIJob's status, we define the following phases:

```go
const (
	// JobCreated means the job has been submitted to the cluster,
	// but not all the pods and services have been created,
	// or no pods are running
	JobCreated Phase = "Created"

	// JobRunning means all the pods are in running state
	JobRunning Phase = "Running"

	// JobSucceeded means job completed without error
	JobSucceeded Phase = "Succeeded"

	// JobFailed means some pods failed, job is also considered failed
	JobFailed Phase = "Failed"

	// JobUnknown means the job is in unknown state
	JobUnknown Phase = "Unknown"
)
```
A normal DIJob that runs and ends successfully will go through three stages, that is Created, Running and Succeeded:
- When DIJob is submitted, di-operator will enter the Created phase after creating the coordinator.
- When the coordinator pod is in the Running phase, DIJob enters the Running phase.
- When the coordinator pod is in the Completed phase, DIJob enters the Succeeded phase.

In addition, when the coordinator pod is in the Failed phase, DIJob will also enter the Failed phase. The aggregators, collectors, and learners will restart immediately after failure, but they will not affect DIJob's phase.

Unknown phase has not been defined.

### Control loop
Built upon [kubebuilder v3](https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.0.0/kubebuilder_linux_amd64), components such as [reflectors, informers, indexers](https://github.com/kubernetes/sample-controller/blob/master/docs/controller-client-go.md) and controllers required by operator are all encapsulated in [manager](https://github.com/kubernetes-sigs/controller-runtime/blob/master/pkg/manager/manager.go) of [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime). Kubebuilder only exposes a common function named `Reconcile` to us to implement reconcile logic for DIJob.
```go
func (r *DIJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // your reconcile logic here
    return ctrl.Result{}, nil
}
```

When DIJob is submitted, we firstly list pods that belong to DIJob in the Reconcile function and find that the coordinator has not been created. Then we read the coordinator template defined in DIJob and create the corresponding coordinator pod (used to run coordinator main process) and service (used for inter-pod communication), and write some environment variables into the pod, including the name of the pod, the namespace of the pod, the port which coordinator listens to, and the URL to access the coordinator.

The port occupied by each module of the DI-engine framework has a default value, as shown below:

```go
DefaultCollectorPort   = 22270
DefaultLearnerPort     = 22271
DefaultAggregatorPort  = 22272
DefaultCoordinatorPort = 22273
```

After the coordinator is created, di-operator will monitor the status of the pod and modify the status of the DIJob. After DIJob is completed (Succeeded or Failed), di-operator will delete all services of the DIJob, and all pods that are in the Running phase of the DIJob by default.

### Webhook
There may be some mistakes when submitting a DIJob, such as spelling mistakes in DIJob's fields, field value miss matched with predefined, etc., resulting in potential errors when managing DIJob's life cycle. For the other hand, it is necessary to set default values for some fields of DIJob. If the default value of DIJob can be set before DIJob is submitted, and a correctness check can be performed, it will help us find problems in advance.

To achieve the above goals, we can configure webhooks in K8s. K8s webhook consists of MutatingWebhook and ValidatingWebhook. The former is used to modify the value of the K8s resource object, and the latter is used to verify the correctness of the K8s resource object.

The webhook verification is implemented in di-operator. MutatingWebhook is created to set the default value for DIJob; ValidatingWebhook is created to verify the correctness of DIJob. For example, for the `CleanPodPolicy` field in DIJob, we set its default value in MutatingWebhook to `Running`, which means that all running pods will be deleted after DIJob is completed. We verify the value of the `CleanPodPolicy` field in ValidatingWebhook, if the value set by the user is not equal to any of `None`, `ALL`, or `Running`, the DIJob will be rejected.

## DI Server
Di-server is an http server customized for DI-engine framework, providing the apis of adding, deleting, and querying collectors, learners, and aggregators. By calling the related apis of di-server, di-server can provide DIJob with the ability to dynamically scale collectors and learners. The following will briefly introduce the design of di-server, including the local cache for storing AggregatorConfig, DIJob and all pods of DIJob; the http interface design for dynamically adding, deleting and querying collectors, learners and aggregators.

### Local cache
In order to reduce the frequency of queries between di-server and K8s api server, thereby reducing the burden of K8s api server, we use [client-go](https://github.com/kubernetes/client-go)'s informer mechanism to store AggregatorConfig, DIJob and all pods of DIJob in local cache, as shown in the following figure

[Schematic diagram](https://github.com/kubernetes/sample-controller/blob/master/docs/controller-client-go.md)

![](images/client-go-controller-interaction.jpeg)

In the above figure, we only pay attention to the upper part. Reflector receives notifications of the existence of new resource instance through list & watch api, and puts the new resource instance into Delta Fifo queue, and informer gets the new resource instance from the Delta Fifo queue and passes it through indexer to store in local cache. The query operation can be completed by querying the local cache, reducing the number of requests to K8s api server. The query command is as following:

```go
genericInformer.Informer().GetIndexer().GetByKey(key)
```

When the resource object changes, the reflector will also receive notifications and update the local cache. In addition, the informer will also periodically synchronize the local cache with K8s api server to be consistent with the resource objects in K8s cluster.


### HTTP interface
In order to support dynamic scaling of collectors/learners for DIJobs, di-server implements some http interfaces for adding, deleting and querying collectors/learners, as shown in the following figure:

![](images/di-api.png)

The following http interfaces are provided:

| method  |  path |  description |
|---|---|---|
| GET  | /v1alpha2/replicas  |  list all collectors and learners |
| GET  | /v1alpha2/replicas?namespace=xxx  | list all collectors and learners in namespace  |
| GET  | /v1alpha2/replicas?namespace=xxx&coordinator=xxx  | list all replicas belongs to coordinator  |
| GET  | /v1alpha2/replicas?namespace=xxx&aggregator=xxx  | get learners belongs to aggregator  |
| DELETE  | /v1alpha2/replicas  | delete some replicas. put data in request body  |
| POST  | /v1alpha2/replicas  | create replicas. put data in request body  |
| POST  | /v1alpha2/replicas/failed  | post failed replicas and request for recreation. put data in request body  |


## Advantages of DI Orchestrator
DI Orchestrator provides a K8s-based container-orchestration solution for the DI-engine framework in a distributed scenario. For a DIJob, di-operator is responsible for arranging the various modules of DI-engine so that each module can run normally and perform training tasks. By calling di-server’s HTTP interface, coordinator is given the ability to add, delete, and query all its collectors, learners, aggregators and improve the dynamic allocation of DI-engine framework resources. In summary, DI Orchestrator provides the following advantages:
1. Encapsulation. Relying on the orchestration capabilities of di-operator, deploying DI-engine distributed RL training (including pod creation and service discovery) is transparent to us. According to the deployment requirements of the DI-engine framework for distributed RL training, di-operator will create coordinator, and then the coordinator will request di-server to create other modules. Di-operator will record the status of the pod of each module into the status of the DIJob. The life cycle of DIJob is also maintained by di-operator, providing us with status of DIJob in different stages.
2. Ease of use. We only need to define the configuration of coordinator, collector, and learner in the yaml file of DIJob, and submit them to K8s cluster with one click. Di-operator will be responsible for deploying DI-engine RL training and liberating us from the complex distributed RL deployments in K8s cluster.
3. Robustness. Relying on the pod restart mechanism of K8s, ensures that pods can automatically restart in the event of an unexpected exit, and the coordinator can respond quickly and reconnect.
4. Dynamic expansion. Collectors/learners required by DIJob are dynamically changing, so di-server provides HTTP interfaces to allow us to dynamically adjust the number of collectors/learners, so that DIJob can adjust the ratio of collectors and learners according to its own needs to optimize throughput.
