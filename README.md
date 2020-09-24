# fortiadc-operator

**Project status: *alpha***. The API, spec, status and other user-facing objects may change in incompatible way.

## Overview

FortiADC Operator creates/configures/manages FortiADC resources from Kubernetes. It allows the dynamic creation of Virtual Server on Kubernetes Service of type LoadBalancer.

## Controllers for Core Resources

The Operator acts on the following core resources:

* **`Node`**: Creates a Kubernetes RealServer object for each Kubernetes node in the cluster.

* **`Service`**: Creates a Kubernetes VirtualServer object for each Kubernetes service of type LoadBalancer with annotation "fortiadc.ouest-france.fr/virtualserver-name".

## CustomResourceDefinitions

The Operator acts on the following [custom resource definitions (CRDs)](https://kubernetes.io/docs/tasks/access-kubernetes-api/extend-api-custom-resource-definitions/):

* **`VirtualServer`**, which creates a FortiADC Virtual Server.

* **`RealServer`**, which creates a FortiADC Real Server.

* **`Pool`**, which creates a FortiADC Real Server Pool with all nodes of the Kubernetes cluster.

## Installation

The recommended way to install the Operator is to use the Helm Chart: [see Chart README](charts/fortiadc-operator/README.md)
