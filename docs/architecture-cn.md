# nervex-operator架构
这里将介绍nerveX on k8s的架构，说明nerveX各个模块在k8s系统上如何被创建、如何相互发现、如何开始训练等。有关nerveX的介绍可参考[nerveX key concepts](https://gitlab.bj.sensetime.com/open-XLab/cell/nerveX/tree/doc/one-week/nervex/docs/source/key_concept)。nervex-operator的架构如下图所示：

![](images/nervex-arch.png)

整体分为两大模块：`nervex-server`和`nervex-operator`。接下来将首先介绍一个nerveX任务提交到k8s之后nervex-operator如何将nerveX的各个模块（在k8s中就是一个[pod](https://kubernetes.io/docs/concepts/workloads/pods/)）创建并启动，然后将对nervex-server和nervex-operator进行介绍。

## 任务创建流程
这里介绍任务创建流程，说明一个nerveX任务在k8s中从创建到执行完成的一整个生命周期
- 编写AggregatorConfig yaml文件，定义Aggregator的模板，该模板将在后面创建NerveXJob的时候用来创建aggregator。
- 编写NerveXJob yaml文件，定义coordinator、collector、learner的模板，提交到k8s集群中。
- nervex-operator监听到NerveXJob的提交，创建coordinator，并为coordinator创建可访问的域名。
- coordinator启动之后按照默认配置向nervex-server请求创建一定数目的collector和learner。
- nervex-server收到coordinator的创建请求后，读取NerveXJob中collector和learner的模板，创建相应数目的collector（上图中Cn）和learner（上图中Lm）并把collector和learner可访问的URL返回给请求方。同时，根据每个learner中申请的GPU数目来决定是否创建aggregator。即当learner申请的GPU数目大于1时创建为该learner创建一个aggregator，否则不创建aggregator。
- coordinator等待collector、learner和aggregator连接上后开始下发任务开始训练。
- 用户可手动向nervex-server发送请求增删collector和learner，coordinator会定期查询其可用的collector和learner数目并决定新建或断开连接。
- 训练结束后，nervex-operator默认会将所有处于Running状态的pod删除，coordinator处于Completed状态，则不会被删除

## nervex-operator
nervex-operator是在一个负责在k8s系统中编排NerveXJob的组件，采用k8s [operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)，通过[controller pattern](https://kubernetes.io/docs/concepts/architecture/controller/)中的控制循环监听k8s集群中NerveXJob的状态，并在有需要的时候对NerveXJob的状态进行修改，使得NerveXJob的实际状态与我们预定义的状态尽可能保持一致。

### API定义
nerveX框架分为4个重要的模块，分别是coordinator、aggregator、collector和learner，根据每个模块的特性，我们定义了两种自定义资源（Custom Resource），分别是NerveXJob和AggregatorConfig。前者用来定义一个RL任务的coordinator、collector和learner运行所需的必备条件，包括镜像、启动命令、所需计算和存储资源、环境变量等；后者用来定义一个RL任务的aggregator运行所需的必备条件。

NerveXJob定义：
```go
type NerveXJobSpec struct {
	// Group is a collection of NerveXJobs
	Group string `json:"group,omitempty"`

	//Priority labels the priority of NerveXJob
	PriorityClassName PriorityClassName `json:"priorityClassName,omitempty"`

	// CleanPodPolicy defines the policy to clean pods after NerveXJob completed
	CleanPodPolicy CleanPodPolicy `json:"cleanPodPolicy,omitempty"`

	// Volumes defines the shared volumes for nerveX components
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	Coordinator CoordinatorSpec `json:"coordinator"`

	Collector CollectorSpec `json:"collector,"`

	Learner LearnerSpec `json:"learner,"`
}
```

AggregatorConfig定义：
```go
type AggregatorConfigSpec struct {
	Aggregator AggregatorSpec `json:"aggregator,"`
}
```

> **为什么aggregator单独定义？**
    aggregator对所有使用nerveX框架进行RL训练的任务都是通用的，因此我们将aggregator定义为一个全局的、共享的资源AggregatorConfig，所有RL任务提交后，nervex-operator都将通过读取集群中唯一的AggregatorConfig来创建aggregator。
### 状态定义
用户提交NerveXJob后，nervex-operator便接管了NerveXJob的生命周期的管理，为了便于用户了解NerveXJob的状态，我们定义了以下阶段（phase）：

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
一个正常运行并结束的NerveXJob会经历Created、Running和Succeeded三个阶段：
- 当NerveXJob提交后，nervex-operator将coordinator和aggregator创建后进入Created阶段
- 当coordinator pod处于Running阶段后NerveXJob进入Running阶段
- 当coordinator pod处于Completed阶段后NerveXJob进入Succeeded阶段。
另外，当coordinator pod处于Failed阶段时，NerveXJob也会进入Failed阶段。而aggregator、collector、learner在失败后会立即重启，不会影响NerveXJob所处的阶段。

Unknown阶段暂时未作定义。

### 控制循环
使用[kubebuilder v3](https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.0.0/kubebuilder_linux_amd64)构建项目，operator所需的[reflector、informer、indexer](https://github.com/kubernetes/sample-controller/blob/master/docs/controller-client-go.md)、controller等组件都由[controller-runtime](https://github.com/kubernetes-sigs/controller-runtime)封装到[manager](https://github.com/kubernetes-sigs/controller-runtime/blob/master/pkg/manager/manager.go)中，将调谐（Reconcile）函数暴露给我们实现调谐逻辑，如下代码所示：
```go
func (r *NerveXJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // your reconcile logic here
    return ctrl.Result{}, nil
}
```

当用户提交NerveXJob后，informer获取到该提交事件后触发handler，之后Reconcile函数被调用；Reconcile函数中调用list pod方法发现coordinator未创建，则读取NerveXJob中关于coordinator的定义模板，创建相应的coordinator pod（coordinator程序在其中运行）和service（用于pod间通信），并将一些环境变量写入pod中，包括pod的名称、pod的命名空间、访问coordinator的URL等环境变量。

其中，nerveX框架的每个模块占用的端口都有一个默认值，如下所示：

```go
DefaultCollectorPort   = 22270
DefaultLearnerPort     = 22271
DefaultAggregatorPort  = 22272
DefaultCoordinatorPort = 22273
```

coordinator创建之后，nervex-operator将监听pod的状态并修改NerveXJob的状态。等到NerveXJob完成后（Succeeded或者Failed），nervex-operator默认会将NerveXJob的所有处于Running阶段的pod和所有的service都删除，coordinator pod会保留。

### webhook
用户提交NerveXJob时，可能存在yaml文件里的某些字段输入错误的问题，导致NerveXJob的运行状态达不到预期，影响用户排查问题；或者需要为NerveXJob的某些字段设置默认值。如果在NerveXJob提交到k8s集群前能为NerveXJob设置默认值，以及做一次正确性校验，有助于用户提前发现问题。

在k8s中，可以配置webhook在NerveXJob提交到k8s集群之前对其进行正确性校验。k8s webhook分为MutatingWebhook和ValidatingWebhook，前者用于修改k8s资源对象的值，后者用于验证k8s资源对象的正确性。

nervex-operator中实现了webhook校验方法，创建MutatingWebhook用于设置NerveXJob的默认值；创建ValidatingWebhook用于校验NerveXJob的正确性。比如对`CleanPodPolicy`字段，我们在MutatingWebhook中设置其默认值为`Running`，表示NerveXJob完成后将Running的pod都删除；我们在ValidatingWebhook中校验`CleanPodPolicy`字段的值，如果用户设置的值不等于`None`、`ALL`、`Running`中的任何一个，则拒绝提交该NerveXJob。

## nervex-server
nervex-server是一个为nerveX框架定制的http服务器，提供新增、删除和查询collector、learner、aggregator的功能。通过调用nervex-server的相关接口，nervex-server为NerveXJob提供了动态增删collector和learner的能力。下面将对nervex-server的设计进行简要介绍，包括存储AggregatorConfig、NerveXJob以及NerveXJob所有pod的本地cache；用于动态新增、删除和查询collector、learner和aggregator的http接口设计。

### 本地cache
为了减少nervex-server与k8s api server之间查询的频率，从而减轻k8s api server的负担，我们利用[client-go](https://github.com/kubernetes/client-go)提供的informer机制将AggregatorConfig、NerveXJob和NerveXJob的所有pod存储在本地cache，如下图所示

[示意图](https://github.com/kubernetes/sample-controller/blob/master/docs/controller-client-go.md)

![](images/client-go-controller-interaction.jpeg)

上图中我们只关注上半部分：reflector通过list & watch接受到新的资源实例存在的通知，就将新资源实例放到Delta Fifo queue中，informer从Delta Fifo queue中获取新资源实例并通过Indexer存放到本地cache中。查询操作都可以通过查询本地cache来完成，减少向k8s api server的请求次数。如下命令：
```go
genericInformer.Informer().GetIndexer().GetByKey(key)
```

当资源对象有变更时，reflector同样会接受到通知并更新本地cache；另外，informer也会定期向api server同步本地cache，与k8s集群中的资源对象保持一致。


### http接口
为了支持NerveXJob动态增删collector/learner的需求，nervex-server提供http接口用于对collector/learner进行新增、删除和查询的功能，如下图所示：

![](images/nervex-api.png)

提供如下接口：

| method  |  path |  description |
|---|---|---|
| GET  | /v1alpha1/replicas  |  list all collectors and learners |
| GET  | /v1alpha1/replicas?namespace=xxx  | list all collectors and learners in namespace  |
| GET  | /v1alpha1/replicas?namespace=xxx&coordinator=xxx  | list all replicas belongs to coordinator  |
| GET  | /v1alpha1/replicas?namespace=xxx&aggregator=xxx  | get learners belongs to aggregator  |
| DELETE  | /v1alpha1/replicas  | delete some replicas. put data in request body  |
| POST  | /v1alpha1/replicas  | create replicas. put data in request body  |
| POST  | /v1alpha1/replicas/failed  | post failed replicas and request for recreation. put data in request body  |

各个接口的请求格式、请求参数、请求body、返回值详见[http接口定义](https://gitlab.bj.sensetime.com/platform/CloudNative4AI/cluster-lifecycle/nervex-operator/issues/6)。

## nervex-operator优势
nervex-operator为nerveX框架提供了分布式场景下基于k8s的容器运行方案。对于用户提交的NerveXJob，nervex-operator负责对nerveX的各个模块进行编排，使得各个模块可以正常运行并执行训练任务。通过调用nervex-server的接口，赋予coordinator新增、删除和查询其所有的collector、learner和aggregator的功能，提升nerveX框架资源动态分配的能力。总结nervex-operator提供了以下优势：
1. 封装性。依赖nervex-operator的编排能力，部署nerveX分布式RL训练的细节（包括pod创建、服务发现）对用户来说是透明的。根据nerveX框架对分布式RL训练的部署需求，nervex-operator会将coordinator创建出来，然后coordinator再请求nervex-server创建其他模块，nervex-operator会把每个模块的pod的状态记录到NerveXJob的状态中。NerveXJob的生命周期也由nervex-operator维护，向用户展示NerveXJob在不同阶段的状态。
2. 易用性。用户只需要在NerveXJob的yaml文件中定义好coordinator、collector、learner的配置之后，一键提交到k8s集群即可，nervex-operator将负责完成部署工作，将用户从k8s集群中复杂的分布式RL训练部署中解放出来。
3. 鲁棒性。依赖k8s的pod重启机制，保证pod在意外退出的情况下能自动重启，coordinator能够迅速响应并重新连接。
4. 动态扩展。NerveXJob所需的collector/learner/aggregator是动态变化的，因此nervex-server提供了http接口可以动态调整collector/learner的数目，使得NerveXJob可以根据自身需求调整collector和learner的比例，优化吞吐量。
