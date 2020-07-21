---
title: Getting Started
description: Getting started with Gloo Federation
weight: 10
---

Gloo Federation enables you to configure and manage multiple Gloo instances in multiple Kubernetes clusters. In this guide, we will walk you through the process of deploying two Kubernetes clusters using [kind](https://kind.sigs.k8s.io/), deploying Gloo and Gloo Federation to those clusters, and registering a cluster with Gloo Federation.

## Prerequisites

To successfully follow this Getting Started guide, you will need the following software available and configured on your system.

* **Docker** - Runs the containers for kind and all pods inside the clusters.
* **Kubectl** - Used to execute commands against the clusters.
* **Kind** - Deploys two Kubernetes clusters using containers running on Docker.
* **Helm** - Used to deploy the Gloo Federation and Gloo charts.
* **Glooctl** - Used to register the Kubernetes clusters with Gloo Federation.

## Deploy the clusters

The first step is to deploy the Kubernetes clusters using kind and `glooctl`. Two clusters will be created, local and remote. The local cluster will house an installation of Gloo as well as the Gloo Federation deployment. The remote cluster will house an installation of Gloo and will need to be registered with Gloo Federation.

You can generate the clusters by running the following command:

```
glooctl demo federation --license <license key>
```

That command will deploy the two Kubernetes clusters, install Gloo on both, and install Gloo Federation on the local cluster. Gloo will be installed in the `gloo-system` namespace, and Gloo Federation will be installed in the `gloo-fed` namespace.

You can check for the clusters by running the following command:

```
kind get clusters
```

```
local
remote
```

Your kubectl context will be set to `kind-local` for the local cluster by default.

You can verify the Gloo installation on each cluster by running the following command:

```
kubectl get deployment -n gloo-system --context kind-local
kubectl get deployment -n gloo-system --context kind-remote
```

```
NAME            READY   UP-TO-DATE   AVAILABLE   AGE
discovery       1/1     1            1           4h1m
gateway         1/1     1            1           4h1m
gateway-proxy   1/1     1            1           4h1m
gloo            1/1     1            1           4h1m
```

You can verify the Gloo Federation installation by running the following command:

```
kubectl get deployment -n gloo-fed --context kind-local
```

```
NAME               READY   UP-TO-DATE   AVAILABLE   AGE
gloo-fed           1/1     1            1           4h4m
gloo-fed-console   1/1     1            1           4h4m
```

You now have Gloo Federation deployed with two Gloo instances in two Kubernetes clusters. The next step is to register the remote Gloo instance with Gloo Federation.

## Register remote cluster

Gloo Federation will not automatically register the Kubernetes cluster it is running on. Both the local cluster and any remote cluster must be registered manually. The registration process will create a service account and cluster role on the target cluster, and store the access credentials in a Kubernetes secret resource in the admin cluster.

The registration is performed by running the following command:

```
glooctl cluster register --cluster-name remote --remote-context kind-remote
```

```
[Registration output]
```

Credentials for the remote cluster are stored in a secret in the gloo-fed namespace as well. The secret name will be the same as the `cluster-name` specified when registering the cluster.

```
 kubectl get secret -n gloo-fed remote
```

```
NAME    TYPE                 DATA   AGE
remote   solo.io/kubeconfig   1      94m
```

In the remote cluster, Gloo Federation has created a service account, cluster role, and role binding. They can be viewed by running the following commands:

```
kubectl get serviceaccount remote -n gloo-system --context kind-remote
kubectl get clusterrole gloo-federation-controller --context kind-remote
kubectl get clusterrolebinding remote-gloo-federation-controller-clusterrole-binding --context kind-remote
```

Once a cluster has been registered, Gloo Federation will automatically discover all instances of Gloo within the cluster. The discovered instances are stored in a Custom Resource of type glooinstances.fed.solo.io in the gloo-fed namespace. You can view the discovered instances by running the following:

```
kubectl get glooinstances -n gloo-fed
```

```
NAME                      AGE
kind-remote-gloo-system   95m
```

You have now successfully deployed Gloo Federation and added a remote cluster to the configuration.

## Next Steps
With a successful deployment of Gloo Federation, now might be a good time to read a bit more about the concepts behind Gloo Federation or you can try out the Service Failover feature.
