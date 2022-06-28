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
$ kubectl create -f config/samples/atari-dqn-tasks.yaml

# get pod and you will see coordinator is created by di-operator
# a few seconds later, you will see collectors and learners created by di-server
$ kubectl get pod
NAME                         READY   STATUS    RESTARTS      AGE
job-with-tasks-collector-0   1/1     Running   0             2s
job-with-tasks-collector-1   1/1     Running   0             2s
job-with-tasks-evaluator-0   1/1     Running   0             2s
job-with-tasks-learner-0     1/1     Running   0             2s

# get logs of tasks
$ kubectl logs job-with-tasks-evaluator-0 
/opt/conda/lib/python3.8/site-packages/torch/cuda/__init__.py:52: UserWarning: CUDA initialization: Found no NVIDIA driver on your system. Please check that you have an NVIDIA GPU and installed a driver from http://www.nvidia.com/Download/index.aspx (Triggered internally at  /opt/conda/conda-bld/pytorch_1607370172916/work/c10/cuda/CUDAFunctions.cpp:100.)
  return torch._C._cuda_getDeviceCount() > 0
[06-28 08:25:29] INFO     Evaluator running on node 1                                                                                                           func.py:58
A.L.E: Arcade Learning Environment (version +a54a328)
[Powered by Stella]
/opt/conda/lib/python3.8/site-packages/ale_py/roms/__init__.py:44: UserWarning: ale_py.roms contains unsupported ROMs: /opt/conda/lib/python3.8/site-packages/AutoROM/roms/{joust.bin, warlords.bin, maze_craze.bin, combat.bin}
  warnings.warn(
[06-28 08:25:46] INFO     Evaluation: Train Iter(0)       Env Step(0)     Eval Reward(-21.000)                                                                  func.py:58
[06-28 08:25:46] WARNING  You have not installed memcache package! DI-engine has changed to some alternatives. 

$ kubectl logs job-with-tasks-learner-0
/opt/conda/lib/python3.8/site-packages/torch/cuda/__init__.py:52: UserWarning: CUDA initialization: Found no NVIDIA driver on your system. Please check that you have an NVIDIA GPU and installed a driver from http://www.nvidia.com/Download/index.aspx (Triggered internally at  /opt/conda/conda-bld/pytorch_1607370172916/work/c10/cuda/CUDAFunctions.cpp:100.)
  return torch._C._cuda_getDeviceCount() > 0
[06-28 08:25:27] INFO     Learner running on node 0
```

## User Guide

Refers to [user-guide](./docs/architecture.md). For Chinese version, please refer to [中文手册](./docs/architecture-cn.md)

## Contributing

Refers to [developer-guide](./docs/developer-guide.md).

Contact us throw <opendilab.contact@gmail.com>
