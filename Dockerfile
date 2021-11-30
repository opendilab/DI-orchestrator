# Build the di-orchestrator binary
FROM registry.sensetime.com/cloudnative4ai/golang:1.15 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN export http_proxy=http://172.16.1.135:3128 && https_proxy=http://172.16.1.135:3128 \
    && go env -w GO111MODULE=on \
    && go env -w GOPROXY=https://goproxy.cn \
    && go mod download

# Copy the go source
COPY cmd/ cmd/
COPY pkg/ pkg/
COPY main.go main.go

# Build orchestrator
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o di-orchestrator ./main.go

# Use distroless as minimal base image to package the di-orchestrator binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM registry.sensetime.com/cloudnative4ai/ubi:v1.0.0
LABEL maintainer="opendilab.contact@gmail.com"
RUN ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime
WORKDIR /
COPY --from=builder /workspace/di-orchestrator .

ENTRYPOINT ["/di-orchestrator"]
