# Build the di-orchestrator binary
FROM golang:1.16 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY pkg/ pkg/
COPY main.go main.go

# Build orchestrator
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o di-orchestrator ./main.go

# Use distroless as minimal base image to package the di-orchestrator binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM redhat/ubi8:latest
LABEL maintainer="opendilab.contact@gmail.com"
RUN ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime
WORKDIR /
COPY --from=builder /workspace/di-orchestrator .

ENTRYPOINT ["/di-orchestrator"]
