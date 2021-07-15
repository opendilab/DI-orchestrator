# E2E Tests
Through the e2e test, we can test the robustness of DI-engine, ensuring that DIJobs can tolerate common exceptions.

## Run
```bash
go test -timeout 20m -cover -v ./e2e --ginkgo.v --shared-volumes-dir /data/nfs/ding --kubeconfig ~/.kube/config
```
- `shared-volumes-dir` represents the shared volumes directory for DI-engine modules (coordinator, collector, etc.) to exchange data and models. Different jobs's shared volumes are placed under this directory. Default `/data/nfs/ding`.
- `kubeconfig` represents path to kubeconfig file to access kubernetes cluster. Default `$HOME/.kube/config`.
- `timeout` can be set according to how long the test will last.
