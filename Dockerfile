# Build nervex-operator with local nervex-operator binary
FROM registry.sensetime.com/cloudnative4ai/ubi:v1.0.0 as dev-nervex-operator
WORKDIR /
COPY /bin/nervex-operator  .

ENTRYPOINT ["/nervex-operator"]

# Build the nervex-operator binary
FROM registry.sensetime.com/cloudnative4ai/golang:1.14 as builder
LABEL maintainer="liqingping@sensetime.com"

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
ARG GOPROXY=https://goproxy.cn
ARG GOPRIVATE=go-sensephoenix.sensetime.com
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY utils/ utils/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o nervex-operator main.go

# Use distroless as minimal base image to package the nervex-operator binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM registry.sensetime.com/cloudnative4ai/ubi:v1.0.0 as nervex-operator
WORKDIR /
COPY --from=builder /workspace/nervex-operator .
RUN ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime

ENTRYPOINT ["/nervex-operator"]
