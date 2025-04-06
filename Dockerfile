# Build the manager binary
FROM --platform=$BUILDPLATFORM golang:1.23.5-alpine as builder

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
COPY internal/ internal/
COPY client/ client/

ARG TARGETOS TARGETARCH
# Build
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GO111MODULE=on go build -a -o manager main.go

FROM alpine:3.18.0
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65534:65534

ENTRYPOINT ["/manager"]
