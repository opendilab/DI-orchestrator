# Build di-operator with local di-operator binary
FROM registry.sensetime.com/cloudnative4ai/ubi:v1.0.0 as dev-di-operator
RUN ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime
WORKDIR /
COPY /bin/di-operator  .

ENTRYPOINT ["/di-operator"]

# Build the di-operator binary
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
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o di-operator main.go

# Use distroless as minimal base image to package the di-operator binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM registry.sensetime.com/cloudnative4ai/ubi:v1.0.0 as di-operator
RUN ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime
WORKDIR /
COPY --from=builder /workspace/di-operator .

ENTRYPOINT ["/di-operator"]
