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

## Usage

This operator allows the automatic creation of FortiADC resources to expose externally a Kubernetes Service of type LoadBalancer.

When the operator is deployed in a cluster, a Kubernetes CR RealServer is created for each Kubernetes Node. Each Kubernetes CR RealServer in turn creates a FortiADC Real Server with the IP of the node.

The operator then watches for Kubernetes Service of type LoadBalancer with a "loadBalancerIP" defined and a "fortiadc.servicecontroller.io/virtualserver-name" annotation:
```
apiVersion: v1
kind: Service
metadata:
  annotations:
    fortiadc.servicecontroller.io/virtualserver-name: MY_VIP
  name: myvip
  namespace: default
spec:
  loadBalancerIP: 192.168.1.50
  ports:
  - name: http
    port: 80
    protocol: TCP
    targetPort: 80
  selector:
    app: myapp
  type: LoadBalancer
```

:warning: **This operator use and export only the first port of the Kubernetes Service, additional ports are not used**

For each Kubernetes Service eligible it creates a Kubernetes CR Pool, which creates a FortiADC Real Server Pool with all Kubernetes Nodes as members and with Kubernetes Node Port as destination port, a Kubernetes CR VirtualServer which creates a FortiADC Virtual Server with created Real Server Pool as destination and the Kubernetes Service port as listening port.

No health checks are created nor set to the Real Server Pool. The operator add/remove FortiADC Real Server and Real Server Pool members on Kubernetes Nodes add/remove. The operator disables FortiADC Real Server when corresponding Kubernetes Node is in status "NotReady" or set as "unschedulable" in Kubernetes Node Spec (for example during drain). The operator enables FortiADC Real Server when the corresponding Kubernetes Node is in status "Ready" and not set as "unschedulable" in Kubernetes Node Spec.
