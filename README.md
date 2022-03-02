[![Build](https://github.com/opendilab/DI-orchestrator/actions/workflows/build.yaml/badge.svg?branch=main)](https://github.com/opendilab/DI-orchestrator/actions/workflows/build.yaml) [![Releases](https://github.com/opendilab/DI-orchestrator/actions/workflows/release.yaml/badge.svg)](https://github.com/opendilab/DI-orchestrator/actions/workflows/release.yaml)
# DI Orchestrator

DI Orchestrator is designed to manage DI ([Decision Intelligence](https://github.com/opendilab/DI-engine/)) jobs using Kubernetes Custom Resource and Operator.

### Prerequisites

- A well-prepared kubernetes cluster. Follow the [instructions](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/create-cluster-kubeadm/) to create a kubernetes cluster, or create a local kubernetes node referring to [kind](https://kind.sigs.k8s.io/docs/user/quick-start/) or [minikube](https://minikube.sigs.k8s.io/docs/start/)

### Install DI Orchestrator

DI Orchestrator consists of two components: `di-operator` and `di-server`. Install them with the following command.

```bash
kubectl create -f ./config/di-manager.yaml
```

`di-operator` and `di-server` will be installed in `di-system` namespace.

```bash
$ kubectl get pod -n di-system
NAME                               READY   STATUS    RESTARTS   AGE
di-operator-57cc65d5c9-5vnvn       1/1     Running   0          59s
di-server-7b86ff8df4-jfgmp         1/1     Running   0          59s
```

### Submit DIJob

```bash
# submit DIJob
$ kubectl create -f config/samples/dijob-gobigger.yaml

# get pod and you will see coordinator is created by di-operator
# a few seconds later, you will see collectors and learners created by di-server
$ kubectl get pod
NAME                READY   STATUS    RESTARTS   AGE
gobigger-test-0-0   1/1     Running   0          4m17s
gobigger-test-0-1   1/1     Running   0          4m17s

# get logs of coordinator
$ kubectl logs -n xlab gobigger-test-0-0
Bind subprocesses on these addresses: ['tcp://10.148.3.4:22270',
'tcp://10.148.3.4:22271']
[Warning] no enough data: 128/0
...
[Warning] no enough data: 128/120
Current Training: Train Iter(0) Loss(102.256)
Current Training: Train Iter(0) Loss(103.133)
Current Training: Train Iter(20)        Loss(28.795)
Current Training: Train Iter(20)        Loss(32.837)
...
Current Training: Train Iter(360)       Loss(12.850)
Current Training: Train Iter(340)       Loss(11.812)
Current Training: Train Iter(380)       Loss(12.892)
Current Training: Train Iter(360)       Loss(13.621)
Current Training: Train Iter(400)       Loss(15.183)
Current Training: Train Iter(380)       Loss(14.187)
Current Evaluation: Train Iter(404)     Eval Reward(-1788.326)
```

## User Guide

Refers to [user-guide](./docs/architecture.md). For Chinese version, please refer to [中文手册](./docs/architecture-cn.md)

## Contributing

Refers to [developer-guide](./docs/developer-guide.md).

Contact us throw <opendilab.contact@gmail.com>
