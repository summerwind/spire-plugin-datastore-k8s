#!/bin/bash

set -e

# Build binary
docker build --target release -t plugin-release:latest .

# Copy binary
mkdir release
cd release
docker create --name plugin-release plugin-release:latest
docker cp plugin-release:/go/src/github.com/summerwind/spire-plugin-datastore-k8s/spire-plugin-datastore-k8s ./
docker rm plugin-release

# Create release file
tar zcf spire-plugin-datastore-k8s-linux-amd64.tar.gz spire-plugin-datastore-k8s

# Checksum
echo "Plugin checksum: $(shasum -a 256 spire-plugin-datastore-k8s | cut -f 1 -d ' ')"
echo "Release checksum: $(shasum -a 256 spire-plugin-datastore-k8s-linux-amd64.tar.gz | cut -f 1 -d ' ')"

# Create manifest file
kustomize build ../manifests/crds > crd.yaml

# Cleanup
rm -rf spire-plugin-datastore-k8s
