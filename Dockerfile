# Build manager with local manager binary
FROM registry.sensetime.com/cloudnative4ai/alpine:3.9 as dev-manager
WORKDIR /
COPY /bin/manager  .

ENTRYPOINT ["/manager"]

# Build the manager binary
FROM registry.sensetime.com/cloudnative4ai/golang:1.14 as builder
LABEL maintainer="liqingping@sensetime.com"

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY utils/ utils/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o manager main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM registry.sensetime.com/cloudnative4ai/alpine:3.9 as manager
WORKDIR /
COPY --from=builder /workspace/manager .

ENTRYPOINT ["/manager"]
