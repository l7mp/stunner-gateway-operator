###########
# Build the manager binary
# The builder runs on the build platform and cross-compiles for the target: Go needs no
# emulation for that, an emulated compiler is an order of magnitude slower.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder
ARG TARGETOS TARGETARCH

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
COPY internal/ internal/
COPY pkg/config/ pkg/config/

# Build
COPY .git ./
COPY Makefile ./
RUN apk add --no-cache git make bash

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH make build-bin

###########
# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder /workspace/bin/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
