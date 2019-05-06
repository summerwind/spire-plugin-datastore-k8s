# Kubernetes datastore plugin for SPIRE server

A SPIRE datastore plugin that enables you to store data using Kubernetes Custom Resource.

## Motivation

- Make running SPIRE Server on Kubernetes simpler
- Enables SPIRE Server to run without managing persistent volumes

## Installation

Download latest binary and install it to the plugin directory.

```
$ curl -L -O https://github.com/summerwind/spire-plugin-datastore-k8s/releases/latest/download/spire-plugin-datastore-k8s-linux-amd64.tar.gz
$ tar zxvf spire-plugin-datastore-k8s.tar.gz
$ mv spire-plugin-datastore-k8s /path/to/plugin
```

## Configuration

The plugin accepts the following configuration options:

| Configuration | Description |
| --- | --- |
| namespace  | Kubernetes namespace to manage custom resources |
| kubeconfig | Path to configuration file to access kubernetes API |

A sample configuration for SPIRE server:

```
plugins {
  DataStore "k8s" {
    plugin_cmd = "/path/to/plugin"
    plugin_data {
      namespace = "spire"
    }
  }
}
```

## Deploying to Kubernetes

You must create *CustomResourceDefinition (CRD)* resources in your Kubernetes cluster before you deploy the plugin. Create CRDs as follows:

```
$ curl -L https://github.com/summerwind/spire-plugin-datastore-k8s/releases/latest/download/crd.yaml | kubectl apply -f -
```

In addition, the Pod of the SPIRE server must have permissions to manage the resources of the CRDs you created. See example manifest below for more information about permissions.

- [spire-server.yaml](manifests/examples/spire-server.yaml)

## Build from soruce

To build a binary from source, first build the container image and get the binary from it.

```
$ git clone https://github.com/summerwind/spire-plugin-datastore-k8s
$ cd spire-plugin-datastore-k8s
$ docker build --target release -t plugin:latest .
$ docker create -n spire plugin:latest
$ docker cp spire:/opt/spire/bin/spire-plugin-datastore-k8s ./
$ docker rm spire
```
