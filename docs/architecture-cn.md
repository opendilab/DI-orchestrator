# DI Orchestrator架构

DI-engine框架v1版本分为3个重要的模块，分别是coordinator、collector和learner。对应DI Orchestrator v1版本。

DI-engine框架v2版本将各个模块进行了整合，使得在同一个worker内可以完成完整的训练过程，当有新的worker加入时也能直接加入而无需重启。本文将针对DI-engine v2版本对DI Orchestrator v2版本进行详细描述。

有关DI-engine的详细介绍可参考[DI-engine developer tutorial](https://opendilab.github.io/DI-engine/tutorial_dev/index.html)。

为了提供DI-engine在Kubernetes（K8s）中运行的支持，我们设计了DI Orchestrator，本文将说明利用DI Orchestrator，DI-engine的组件在K8s系统上如何被创建、如何相互发现、如何开始训练等。DI Orchestrator的架构如下图所示：

![](images/di-engine-arch.png)

整体分为两大模块：`di-server`和 `di-operator`。本文将对这两大模块逐一进行介绍。

## DI Operator

di-operator是负责在K8s系统中编排DIJob，采用K8s [operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)，通过[controller pattern](https://kubernetes.io/docs/concepts/architecture/controller/)中的控制循环监听K8s集群中DIJob的状态，并在有DIJob状态变更事件的时候对DIJob进行调谐，使得DIJob的实际状态与预期的状态尽可能保持一致。

### API定义

根据DI-engine框架的特性，我们利用K8s Custom Resource定义了DIJob资源，用来定义一个RL任务运行所期望达成的状态，包括镜像、启动命令、挂载存储、workers数目等。

DIJobSpec中各字段定义及含义：

```go
type DIJobSpec struct {
	// Group is a collection of DIJobs.
	Group string `json:"group,omitempty"`

	// Priority labels the priority of DIJob.
	Priority Priority `json:"priority,omitempty"`

	// EngineFields defines features of the DI-engine framework.
	EngineFields EngineFields `json:"engineFields,omitempty"`

	// CleanPodPolicy defines the policy to clean pods after DIJob completed.
	CleanPodPolicy CleanPodPolicy `json:"cleanPodPolicy,omitempty"`

	// Preemptible defines whether the dijob can be preempted.
	Preemptible bool `json:"preemptible,omitempty"`

	// MinReplicas defines the minimum number of replicas of DIJob.
	MinReplicas int32 `json:"minReplicas,omitempty"`

	// MaxReplicas defines the maximum number of replicas of DIJob.
	MaxReplicas int32 `json:"maxReplicas,omitempty"`

	// Template defines the pod template for DIJob.
	Template corev1.PodTemplateSpec `json:"template"`
}

type EngineFields struct {
	// Topology defines the topology among the workers of the job.
	Topology Topology `json:"topology,omitempty"`

	// ParallelWorkers defines the number of parallel workers in each worker.
	ParallelWorkers int32 `json:"parallelWorkers,omitempty"`
}
```

### 状态定义

用户提交DIJob后，di-operator便接管了DIJob的生命周期的管理，我们定义了以下阶段（phase）便于用户了解DIJob的状态：

```go
const (
	// JobPending means the job has been submitted to the cluster,
	// but not all the pods and services have been created
	JobPending Phase = "Pending"

	// JobStarted means the job has been created and waits for running.
	JobStarting Phase = "Starting"

	// JobRestarting means the job has been rescheduled and waits for restarting.
	JobRestarting Phase = "Restarting"

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

一个正常运行并结束的DIJob会经历Pending、Starting、Running和Succeeded三个阶段，状态转移图如下图所示：
![](images/di-engine-status-machine.png)

- 当DIJob提交后，进入Pending阶段。
- 当di-operator将workers创建后，进入Starting状态。
- 当所有workers都ready后，进入Running状态。
- 当所有workers都Succeeded后，进入Succeeded状态。
- 当有worker出现Failed，进入Failed状态。
- 当DIJob被重调度或者workers数目与预期不符，进入Restarting状态。

Unknown阶段暂时未作定义。

### 控制循环
借鉴自[Adaptdl](https://github.com/petuum/adaptdl)，v2版本架构对Operator调谐逻辑进行了重构，将调度和调谐逻辑分别在Allocator和Controller中完成，使得组件分工更明确。
#### Allocator控制循环
Allocator为v2架构中新增的模块，用于调度DIJob，包括分配workers和放置workers。定义两个方法（allocate和allocateAll）用于对单任务和多任务进行调度。为了提供不同的调度策略，我们将调度策略定义为一个interface Policy，该interface中定义了两个方法分别是`Allocate`和`Optimize`，前者用于在任务提交时为该任务进行初始调度；后者用于对全局任务进行统一调度。
Policy interface定义如下：

```go
type Policy interface {
	Allocate(job JobInfo, nodes map[string]*NodeInfo) (NodeList, error)
	Optimize(jobs map[string]JobInfo, nodes map[string]*NodeInfo, prevAllocations map[string]NodeList) (map[string]NodeList, error)
}
```
用户可根据自身需求实现自己的调度算法。

当`job.spec.preemptible==false`时，Allocator将不会对该任务进行调度，只会根据`job.spec.minReplicas`为该任务分配固定数目的workers，分配结果写到`job.status.replicas`。不过，用户可以通过修改`job.status.replicas`来变更该任务的workers数目。
> Note：不能直接通过`kubectl apply`或者`kubectl edit`命令直接修改`job.status.replicas`，因为`job.status`被定义为SubResource，对于DIJob的所有的PUT和POST请求都会忽略`job.status`字段。见[Kubernetes API Conversion](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status)
#### Controller控制循环
Controller控制循环用于调谐DIJob的状态，包括生命周期管理、workers的创建和删除等，如前文所述状态转移图。

## DI Server

di-server是一个为DI-engine框架定制的http服务器，提供新增、删除和查询workers的功能。di-server利用[gin](https://github.com/gin-gonic/gin) web框架提供http服务能力

下面将对di-server的设计进行简要介绍，包括用于动态新增、删除和查询workers的http接口以及用户汇报训练任务profilings数据的接口。

### http接口

为了支持DIJob动态增删collector/learner的需求，di-server提供http接口用于对collector/learner进行新增、删除和查询的功能，如下图所示：


提供如下接口：

| method | path                                             | description                                                               |
| ------ | ------------------------------------------------ | ------------------------------------------------------------------------- |
| GET    | /v2alpha1/replicas                               | list all collectors and learners                                          |
| GET    | /v2alpha1/replicas?namespace=xxx                 | list all collectors and learners in namespace                             |
| GET    | /v2alpha1/replicas?namespace=xxx&coordinator=xxx | list all replicas belongs to coordinator                                  |
| GET    | /v2alpha1/replicas?namespace=xxx&aggregator=xxx  | get learners belongs to aggregator                                        |
| DELETE | /v2alpha1/replicas                               | delete some replicas. put data in request body                            |
| POST   | /v2alpha1/replicas                               | create replicas. put data in request body                                 |
| POST   | /v2alpha1/replicas/failed                        | post failed replicas and request for recreation. put data in request body |

## DI Orchestrator的优势

DI Orchestrator为DI-engine框架提供了分布式场景下基于K8s的容器运行方案。对于用户提交的DIJob，di-operator负责对DI-engine的各个模块进行编排，使得各个模块可以正常运行并执行训练任务。通过调用di-server的接口，赋予coordinator新增、删除和查询其所有的collector、learner和aggregator的功能，提升DI-engine框架资源动态分配的能力。总结DI Orchestrator提供了以下优势：

1. 封装性。依赖di-operator的编排能力，部署DI-engine分布式RL训练的细节（包括pod创建、服务发现）对用户来说是透明的。根据DI-engine框架对分布式RL训练的部署需求，di-operator会将coordinator创建出来，然后coordinator再请求di-server创建其他模块，di-operator会把每个模块的pod的状态记录到DIJob的状态中。DIJob的生命周期也由di-operator维护，向用户展示DIJob在不同阶段的状态。
2. 易用性。用户只需要在DIJob的yaml文件中定义好coordinator、collector、learner的配置之后，一键提交到K8s集群即可，di-operator将负责完成部署工作，将用户从K8s集群中复杂的分布式RL训练部署中解放出来。
3. 鲁棒性。依赖K8s的pod重启机制，保证pod在意外退出的情况下能自动重启，coordinator能够迅速响应并重新连接。
4. 动态扩展。DIJob所需的collector/learner/aggregator是动态变化的，因此di-server提供了http接口可以动态调整collector/learner的数目，使得DIJob可以根据自身需求调整collector和learner的比例，优化吞吐量。
