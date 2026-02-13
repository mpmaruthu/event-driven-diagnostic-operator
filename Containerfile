# Build Stage
FROM golang:1.21 as builder

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod

# Copy the go source (needed for go mod tidy to work)
COPY cmd/ cmd/
COPY internal/ internal/

# Generate go.sum and download dependencies
RUN go mod tidy && go mod download

# Build
# -a: force rebuilding of packages that are already up-to-date.
# -o manager: output binary name
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager cmd/main.go

# Run Stage (Distroless or UBI Minimal)
# We use Red Hat UBI minimal because it fits the OpenShift ecosystem well
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]