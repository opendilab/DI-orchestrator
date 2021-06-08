# nerveX-k8s架构
这里将介绍nerveX on k8s的架构，说明nerveX各个模块在k8s系统上如何被创建、如何相互发现、如何开始训练等。有关nerveX的介绍可参考[nerveX key concepts](https://gitlab.bj.sensetime.com/open-XLab/cell/nerveX/tree/doc/one-week/nervex/docs/source/key_concept)。nerveX-k8s的架构如下图所示：
![](images/nervex-arch10.png)

整体分为两大模块：`nervex-server`和`nervex-operator`。接下来将首先介绍一个nerveX任务提交到k8s之后nervex-k8s如何将nerveX的各个模块（在k8s中就是一个[pod](https://kubernetes.io/docs/concepts/workloads/pods/)）创建并启动，然后将对nervex-server和nervex-operator进行介绍。

## 任务创建流程
这里介绍任务创建流程，说明一个nerveX任务在k8s中从创建到执行完成的一整个生命周期
- 编写AggregatorConfig yaml文件，定义Aggregator的模板，该模板将在后面创建NerveXJob的时候用来创建aggregator
- 编写NerveXJob yaml文件，定义coordinator、collector、learner的模板，提交到k8s集群中
- nervex-operator监听到NerveXJob的提交事件，将coordinator（上图中Coord）和aggregator（上图中Agg）创建出来，并为coordinator和aggregator创建可相互访问的域名
- coordinator启动之后按照默认配置向nervex-server请求创建一定数目的collector和learner
- nervex-server收到coordinator的创建请求后，读取NerveXJob中collector和learner的模板，创建相应数目的collector（上图中Cn）和learner（上图中Lm）并把collector和learner可访问的URL返回给请求方
- coordinator等待collector和learner连接上后开始下发任务开始训练
- 用户可手动向nervex-server发送请求增删collector和learner，coordinator会定期查询其可用的collector和learner数目并决定新建或断开连接
- 训练结束后，nervex-operator默认会将所有处于Running状态的pod删除，coordinator处于Completed状态，则不会被删除

## nervex-operator
nervex-operator是在一个负责在k8s系统中编排NerveXJob的组件，采用k8s [operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)，通过[controller pattern](https://kubernetes.io/docs/concepts/architecture/controller/)中的控制循环监听k8s集群中NerveXJob的状态，并在有需要的时候对NerveXJob的状态进行修改，使得NerveXJob的实际状态与我们预定义的状态尽可能保持一致。

### API定义

### 状态定义

### 控制循环
使用[kubebuilder v3](https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.0.0/kubebuilder_linux_amd64)构建项目，operator所需的[reflector、informer、indexer](https://github.com/kubernetes/sample-controller/blob/master/docs/controller-client-go.md)、controller等组件都由[controller-runtime](https://github.com/kubernetes-sigs/controller-runtime)封装到[manager](https://github.com/kubernetes-sigs/controller-runtime/blob/master/pkg/manager/manager.go)中，将调谐（Reconcile）函数暴露给我们实现调谐逻辑，如下代码所示：
```go
func (r *NerveXJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // your reconcile logic here
    return ctrl.Result{}, nil
}
```

当用户提交NerveXJob后，informer获取到该提交事件后触发handler，调用Reconcile函数；调用list pod方法发现coordinator和aggregator未创建，便读取AggregatorConfig中
### Webhook