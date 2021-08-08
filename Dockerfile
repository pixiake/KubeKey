# Build the manager binary
FROM golang:1.14 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

RUN git clone https://github.com/kubesphere/helm-charts.git
# Copy the go source
ADD ./ /workspace
# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o manager main.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o kk cmd/kk/main.go

# Build the manager image
FROM debian:stable

RUN useradd -m kubekey && apt-get update && apt-get install bash curl -y; apt-get autoclean; rm -rf /var/lib/apt/lists/*

USER kubekey:kubekey
RUN mkdir -p /home/kubekey/kubekey

WORKDIR /home/kubekey

COPY --from=builder /workspace/helm-charts/src/main/nfs-client-provisioner /home/kubekey/addons/nfs-client-provisioner
COPY --from=builder /workspace/helm-charts/src/test/ks-installer /home/kubekey/addons/ks-installer
COPY --from=builder /workspace/manager /home/kubekey
COPY --from=builder /workspace/kk /home/kubekey

