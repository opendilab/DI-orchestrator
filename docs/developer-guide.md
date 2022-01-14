# Developer Guide

## Prerequisites

- a well prepared kubernetes cluster. Follow the [instructions](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/create-cluster-kubeadm/) to create a kubernetes cluster, or create a local kubernetes node referring to [kind](https://kind.sigs.k8s.io/docs/user/quick-start/) or [minikube](https://minikube.sigs.k8s.io/docs/start/)
- kustomize. Installed by the following command

```bash
curl -s "https://raw.githubusercontent.com/\
kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"  | bash
```

## CRD Design

Update codes in [dijob_types.go](./api/v1alpha2/dijob_types.go) with your requirements, and generate deepcopy functions.

```bash
make generate
```

Generate new CRD files with the following command.

```bash
make manifests
```

New CRD files will be generated in [./config/crd/bases](./config/crd/bases)

## Controller Logic

Referenced to [controllers](./controllers)

## DI Server Logic

Referenced to [server](./server)

## Installation

Run the following command in the project root directory.

```bash
# build images. 
make docker-build
make docker-push
# deploy di-operator and server to cluster
make dev-deploy
```

Since the CustomResourceDefinitions are too long, you will probably find the following error:

```bash
The CustomResourceDefinition "dijobs.diengine.opendilab.org" is invalid: metadata.annotations: Too long: must have at most 262144 bytes
```

Then running the following command will solve the problem:

```bash
kustomize build config/crd | kubectl create -f -
```

`di-operator` and `di-server` will be installed in `di-system` namespace.

```bash
$ kubectl get pod -n di-system
NAME                               READY   STATUS    RESTARTS   AGE
di-operator-57cc65d5c9-5vnvn       1/1     Running   0          59s
di-server-7b86ff8df4-jfgmp         1/1     Running   0          59s
```

## Programming Specification

- Logger: logger should use `github.com/go-logr/logr.Logger`, created from `sigs.k8s.io/controller-runtime/pkg/log.DelegatingLogger`. We have the following specifications
  - Logger used in each function should be defined as: `logger := ctx.Log.WithName(function-name).WithValues("job", job-namespace-name))`. It's helpful for debugging since we can easily locate where the log message is from and what the DIJob is. Then, DIJob related information is not needed in log message.
  - All the log message should start with lower case letter.
