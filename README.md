# DI Orchestrator
DI Orchestrator is designed to manage DI (Decision Intelligence) jobs using Kubernetes Custom Resource and Operator. 

### Prerequisites
- a well prepared kubernetes cluster. Follow the [instructions](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/create-cluster-kubeadm/) to create a kubernetes cluster, or create a local kubernetes node referring to [kind](https://kind.sigs.k8s.io/docs/user/quick-start/) or [minikube](https://minikube.sigs.k8s.io/docs/start/)
- cert-manager. Installation on kubernetes referenced to [cert-manager docs](https://cert-manager.io/docs/installation/kubernetes/). Or you can install by the following command.
```bash
kubectl create -f ./config/certmanager/cert-manager.yaml
```

### Install DI Orchestrator
Install `di-operator` and `di-server` with the following command.
```bash
kubectl create -f ./config/di-manager.yaml
```

`di-operator` and `di-server` will be installed in `di-system` namespace. 
```bash
$ kubectl get pod -n -system
NAME                               READY   STATUS    RESTARTS   AGE
di-operator-57cc65d5c9-5vnvn   1/1     Running   0          59s
di-server-7b86ff8df4-jfgmp     1/1     Running   0          59s
```

Install global components of DIJob defined in AggregatorConfig:
```bash
kubectl create -f examples/di_v1alpha1_agconfig.yaml -n di-system
```
### Submit DIJob
```bash
# submit DIJob
$ kubectl create -f examples/di_v1alpha1_dijob.yaml

# get pod and you will see coordinator is created by di-operator
# few seconds later, you will see collectors and learners created by di-server
$ kubectl get pod

# get logs of coordinator
$ kubectl logs dijob-example-coordinator
```

## User Guide
Refers to [user-guide](./docs/architecture.md). For Chinese version, please refer to [中文手册](./docs/architecture-cn.md)

## Contributing
Refers to [developer-guide](./docs/developer-guide.md). Contact us throw <opendilab.contact@gmail.com>