# 开发手册
## 项目初始化（已经完成，不需要再做）
kubebuilder需安装v3版本，由于目前v3.0.0还处于预发版阶段，暂时用[v3.0.0-beta.0](https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.0.0-beta.0/kubebuilder_linux_amd64)，v2版本生成的CRD与k8s v1.20不匹配，没办法提交到集群中去
```bash
kubebuilder init --domain sensetime.com --license apache2 --owner "The SensePhoenix authors"

kubebuilder create api --group nervex --version v1alpha1 --kind NervexJob

kubebuilder create api --group nervex --version v1alpha1 --kind ActorLearnerConfig
```

## CRD设计
编写[nervexjob_types.go](./api/v1alpha1/nervexjob_types.go)和[actorlearnerconfig_types.go](./api/v1alpha1/actorlearnerconfig.go)之后，重新生成deepcopy函数：
```bash
make generate
```
生成CRD文件：
```bash
make manifests
```
新生成的CRD文件会出现在[./config/crd/bases](./config/crd/bases)目录

## controller逻辑设计
修改[controllers](./controllers)目录下的代码，修改controller逻辑

## nervex-server逻辑设计
修改[server](./server)目录下的代码，修改nervex-server逻辑