# fortiadc-operator

Fortiadc-operator is an operator which allow to create external resource in a fortiadc server

## Prerequisites

- Kubernetes 1.15+
- Helm 3.1+

## Installing the Chart

If not already done you first have to add the chart repo:

```console
$ helm repo add ouestfrance https://charts.ouest-france.fr/
```

To install the chart with the release name `fortiadc-operator`, first create a namespace:

```console
$ kubectl create namespace fortiadc-operator
```

Then create the helm release

```console
$ helm upgrade -i fortiadc-operator -n fortiadc-operator ouestfrance/fortiadc-operator
```

The command deploys the operator on the Kubernetes cluster in the default configuration. You need to provide the credentials to access to your
fortiadc server.

## Uninstalling the Chart

To uninstall/delete the `fortiadc-operator` deployment:

```console
$ helm delete fortiadc-operator
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Parameters

The following table lists the configurable parameters of the fortiadc-operator chart and their default values.

|            Parameter                |         Description                                  |                  Default               |
| ----------------------------------- | ---------------------------------------------------- | -------------------------------------- |
| `affinity`                          | pod affinity                                         |  `{}`                                  |
| `fullnameOverride`                  | chart fullname override                              |  ``                                    |
| `fortiadc.secret.address`           | Adress of your fortiadc server                       |  ``                                    |
| `fortiadc.secret.insecure`          | Set to false for http                                |  `true`                                |
| `fortiadc.secret.password`          | Password for fortiadc api access                     |  ``                                    |
| `fortiadc.secret.username`          | Username for fortiadc api access                     |  ``                                    |
| `image.repository`                  | fortiadc-operator image repository                   | `ouestfrance/fortiadc-operator`        |
| `image.tag`                         | fortiadc-operator image tag                          | `0.1.4`                                |
| `image.pullPolicy`                  | fortiadc-operator image pull policy                  | `Always`                               |
| `nodeSelector`                      | pod node selector                                    | `{}`                                   |
| `replicaCount`                      | numer of pod replicas                                | `2`                                    |
| `resources.limits.cpu`              | fortiadc-operator container cpu limit                | `100m`                                 |
| `resources.limits.memory`           | fortiadc-operator container memory limit             | `30Mi`                                 |
| `resources.requests.cpu`            | fortiadc-operator container cpu request              | `100m`                                 |
| `resources.requests.memory`         | fortiadc-operator container memory request           | `20Mi`                                 |
| `serviceAccount.create`             | Specifies whether a ServiceAccount should be created | `true`                                 |
| `serviceAccount.name`               | The name of the service account to use               | ``                                     |
| `serviceMonitor.enabled`            | If true, a ServiceMonitor resource is created        | `false`                                |
| `service.type`                      | type of kubernetes service                           | `ClusterIP`                            |
| `service.port`                      | port of service                                      | `443`                                  |
| `tolerations`                       | pod toleration                                       | `[]`                                   |
