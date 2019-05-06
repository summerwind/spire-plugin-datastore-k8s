FROM golang:1.12-alpine AS base

ENV GO111MODULE=on \
    GOPROXY=https://proxy.golang.org

ENV KUBEBUILDER_VERSION=1.0.8

RUN apk add curl ca-certificates build-base git mercurial

RUN curl -L -O https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_${KUBEBUILDER_VERSION}_linux_amd64.tar.gz \
  && tar zxvf kubebuilder_${KUBEBUILDER_VERSION}_linux_amd64.tar.gz \
  && mv kubebuilder_${KUBEBUILDER_VERSION}_linux_amd64 /usr/local/kubebuilder

RUN go get k8s.io/code-generator/cmd/deepcopy-gen \
  && install -o root -g root -m 755 ${GOPATH}/bin/deepcopy-gen /usr/local/bin/deepcopy-gen

###################

FROM base AS builder

WORKDIR /go/src/github.com/summerwind/spire-plugin-datastore-k8s
COPY go.mod go.sum .
RUN go mod download

COPY . .

RUN go vet .
RUN go test -v .
RUN go build .

###################

FROM gcr.io/spiffe-io/spire-server:0.7.3

COPY --from=builder /go/src/github.com/summerwind/spire-plugin-datastore-k8s/spire-plugin-datastore-k8s /opt/spire/bin/spire-plugin-datastore-k8s
