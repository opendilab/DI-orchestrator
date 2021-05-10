# Build the coordinator binary
FROM registry.sensetime.com/cloudnative4ai/golang:1.14 as builder

WORKDIR /workspace

# Copy the go source
COPY main.go main.go


# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o coordinator main.go

# Use distroless as minimal base image to package the coordinator binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM registry.sensetime.com/cloudnative4ai/ubi:v1.0.0
WORKDIR /
COPY --from=builder /workspace/coordinator .

ENTRYPOINT ["/coordinator"]

# docker build -t registry.sensetime.com/cloudnative4ai/liqingping/coordinator:v0.0.1 .