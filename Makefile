
# di-operator version
VERSION ?= v0.2.0
MASTER_VERSION := $(VERSION)

COMMIT_SHORT_SHA=$(shell git log -n 1 | head -n 1 | sed -e 's/^commit //' | head -c 8)

VERSION := $(VERSION)-${COMMIT_SHORT_SHA}

ifeq ($(GIT_BRANCH),master)
VERSION := $(MASTER_VERSION)
endif

ifneq ($(findstring release,$(GIT_BRANCH)),)
VERSION := $(MASTER_VERSION)
endif

# Image URL to use all building/pushing image targets
IMG_BASE ?= opendilab/di-operator
SERVER_IMG_BASE ?= opendilab/di-server
WEBHOOK_IMG_BASE ?= opendilab/di-webhook

IMG ?= ${IMG_BASE}:${VERSION}
MASTER_IMG ?= ${IMG_BASE}:${MASTER_VERSION}

SERVER_IMG ?= ${SERVER_IMG_BASE}:${VERSION}
MASTER_SERVER_IMG ?= ${SERVER_IMG_BASE}:${MASTER_VERSION}

WEBHOOK_IMG ?= ${WEBHOOK_IMG_BASE}:${VERSION}
MASTER_WEBHOOK_IMG ?= ${WEBHOOK_IMG_BASE}:${MASTER_VERSION}

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=di-operator-cluster-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	cd config/manager && $(KUSTOMIZE) edit set image ${IMG_BASE}=${MASTER_IMG} ${SERVER_IMG_BASE}=${MASTER_SERVER_IMG} ${WEBHOOK_IMG_BASE}=${MASTER_WEBHOOK_IMG}
	./hack/update-image-tags.sh config/manager ${MASTER_VERSION}
	./hack/update-version.sh ${MASTER_VERSION}
## generate installer scripts
	$(KUSTOMIZE) build config/default > config/di-manager.yaml

# dev-manifests will add COMMIT_SHORT_SHA to ci version, and image tag, so it is only used for development
# used `make manifests` when commited git
dev-manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=di-operator-cluster-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	cd config/manager && $(KUSTOMIZE) edit set image ${IMG_BASE}=${IMG} ${SERVER_IMG_BASE}=${SERVER_IMG} ${WEBHOOK_IMG_BASE}=${WEBHOOK_IMG}
	./hack/update-image-tags.sh config/manager ${VERSION}
	./hack/update-version.sh ${VERSION}

generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

# Run golangci-lint
lint:
	golangci-lint run -v --timeout=5m

.PHONY: test
test: ginkgo ## Run tests.
	$(GINKGO) -nodes 4 -v -cover -coverprofile=coverage.out ./api/v1alpha1 ./controllers ./server/http ./common/gpuallocator 
	go tool cover -func=./api/v1alpha1/coverage.out 
	go tool cover -func=./controllers/coverage.out 
	go tool cover -func=./server/http/coverage.out 
	go tool cover -func=./common/gpuallocator/coverage.out

##@ Build

build: generate  ## Build di-operator binary.
	go build -o bin/di-operator cmd/operator/main.go
	go build -o bin/di-server cmd/server/main.go
	go build -o bin/di-webhook cmd/webhook/main.go

run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/operator/main.go

operator-image:
	docker build -t ${IMG} --target di-operator .

webhook-image:
	docker build -t ${WEBHOOK_IMG} --target di-webhook .

server-image:
	docker build -t ${SERVER_IMG} --target di-server .

docker-build: operator-image webhook-image server-image ## Build docker image with the di-operator.

dev-images: build
	docker build -t ${IMG} -f Dockerfile.dev --target di-operator .
	docker build -t ${WEBHOOK_IMG} -f Dockerfile.dev --target di-webhook .
	docker build -t ${SERVER_IMG} -f Dockerfile.dev --target di-server .

docker-push: ## Push docker image with the di-operator.
	docker push ${IMG}
	docker push ${SERVER_IMG}
	docker push ${WEBHOOK_IMG}

docker-release: ## Release docker image with the di-operator.
	docker pull ${IMG}
	docker pull ${SERVER_IMG}
	docker pull ${WEBHOOK_IMG}
	docker tag ${IMG} ${MASTER_IMG}
	docker tag ${SERVER_IMG} ${MASTER_SERVER_IMG}
	docker tag ${WEBHOOK_IMG} ${MASTER_WEBHOOK_IMG}
	docker push ${MASTER_IMG} 
	docker push ${MASTER_SERVER_IMG}
	docker push ${MASTER_WEBHOOK_IMG}

##@ Deployment

install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl apply -f -

dev-deploy: dev-manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl apply -f -

undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete -f -

dev-undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete -f -

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.1)

KUSTOMIZE = $(shell pwd)/bin/kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v3@v3.8.7)

GINKGO = $(shell pwd)/bin/ginkgo
ginkgo: ## Download ginkgo locally if necessary.
	$(call go-get-tool,$(GINKGO),github.com/onsi/ginkgo/ginkgo@v1.14.1)

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef
