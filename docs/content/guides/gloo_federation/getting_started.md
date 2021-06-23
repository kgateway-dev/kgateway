---
title: Getting Started
description: Getting started with Gloo Edge Federation
weight: 10
---

Gloo Edge Federation enables you to configure and manage multiple Gloo Edge instances in multiple Kubernetes clusters. In this guide, we use `glooctl` to create a demonstration environment running Gloo Edge Federation.

## Prerequisites

To successfully follow this Getting Started guide, you will need the following software available and configured on your system.

* **Docker** - Runs the containers for kind and all pods inside the clusters.
* **Kubectl** - Used to execute commands against the clusters.
* **[Kind](https://kind.sigs.k8s.io/)** - Deploys two Kubernetes clusters using containers running on Docker.
* **Helm** - Used to deploy the Gloo Edge Federation and Gloo Edge charts.
* **Glooctl** - Used to deploy the demonstration environment.

{{% notice note %}}
Gloo Edge Enterprise version >= 1.5.0-beta4 is needed for failover.
If you are using the demo command, that uses the latest version by default.
{{% /notice %}}

## Upgrading Gloo Edge to use failover

Failover can be enabled by setting following helm value: `gatewayProxies.NAME.failover.enabled=true`.

An example Helm override file for installing Gloo Edge with failover is:
```yaml
gatewayProxies:
  gatewayProxy:
    failover:
      enabled: true
    service:
      type: NodePort
```

An example helm command to upgrade Gloo Edge is:
```
helm upgrade gloo gloo/gloo --namespace gloo-system --values enable-failover.yaml
```

## Deploy the demonstration environment

We will use the `demo` command from  `glooctl` to set up the environment. The end result will be a fully functioning local environment running two Kubernetes clusters, Gloo Edge Enterprise, and Gloo Edge Federation. 

You can generate the demo environment by running the following command:

```
glooctl demo federation --license-key <license key>
```

You will need a license key to deploy the demonstration environment. You can request a key by visiting the [Solo.io website](https://solo.io).

That command performs the following actions: 

1. Deploy two kind clusters called local and remote
1. Install Gloo Edge Enterprise on both clusters in the gloo-system namespace
1. Install Gloo Edge Federation on the local cluster in the gloo-system namespace
1. Register both Gloo Edge Enterprise instances with Gloo Edge Federation
1. Created federated configuration resources
1. Create a Failover Service configuration using both Gloo Edge Enterprise instance

Once the demo environment has completed provisioning, we can explore the environment in the following sections.

## Exploring the demo environment

The local demo environment is a sandbox for you to explore the functionality of Gloo Edge Federation. Let's take a look at what has been deployed.

### Kubernetes clusters and Gloo Edge installations

You can view the clusters by running the following command:

```
kind get clusters
```

```
local
remote
```

You will have two new kubectl contexts, kind-local and kind-remote. Your kubectl context will be set to `kind-local` for the local cluster by default.

You can verify the Gloo Edge installation on each cluster by running the following command:

```
kubectl get deployment -n gloo-system --context kind-local
kubectl get deployment -n gloo-system --context kind-remote
```

```
NAME            READY   UP-TO-DATE   AVAILABLE   AGE
discovery       1/1     1            1           8m45s
gateway         1/1     1            1           8m45s
gateway-proxy   1/1     1            1           8m45s
gloo            1/1     1            1           8m45s
rate-limit      1/1     1            1           8m45s
redis           1/1     1            1           8m45s
```

You can verify the Gloo Edge Federation installation by running the following command:

```
kubectl get deployment -n gloo-system --context kind-local
```

```
NAME               READY   UP-TO-DATE   AVAILABLE   AGE
gloo-fed           1/1     1            1           24m
gloo-fed-console   1/1     1            1           24m
```

## Cluster registration

Kubernetes clusters running Gloo Edge Enterprise must be registered with Gloo Edge Federation to be managed. Once registered, Gloo Edge Federation will automatically discover all instances of Gloo Edge running on the cluster. The `glooctl federation demo` command took care of the registration process for us. The registration creates a service account, cluster role, and cluster role binding on the target cluster, and stores the access credentials in a Kubernetes secret resource in the admin cluster.

Credentials for the target cluster are stored in a secret in the gloo-system namespace. The secret name will be the same as the `cluster-name` specified when registering the cluster. Let's take a look at the secret for the remote cluster.

```
kubectl get secret -n gloo-system kind-remote
```

```
NAME          TYPE                 DATA   AGE
kind-remote   solo.io/kubeconfig   1      2m53s
```

In the target cluster, Gloo Edge Federation has created a service account, cluster role, and role binding. They can be viewed by running the following commands:

```
kubectl get serviceaccount kind-remote -n gloo-system --context kind-remote
kubectl get clusterrole gloo-federation-controller --context kind-remote
kubectl get clusterrolebinding kind-remote-gloo-federation-controller-clusterrole-binding --context kind-remote
```

Once a cluster has been registered, Gloo Edge Federation will automatically discover all instances of Gloo Edge within the cluster. The discovered instances are stored in a Custom Resource of type glooinstances.fed.solo.io in the gloo-system namespace. The naming of each resource will follow the convention `clustername-gloo-namespace`. 

You can view the discovered instances by running the following:

```
kubectl get glooinstances -n gloo-system
```

```
NAME                      AGE
kind-local-gloo-system    4m33s
kind-remote-gloo-system   4m1s
```

### Federated Configuration

Gloo Edge Federation enables you to create consistent configurations across multiple Gloo Edge instances. The resources being configured could be resources such as Upstreams, UpstreamGroups, Virtual Services. Gloo Edge Federation has federated versions as Custom Resource Definitions, like FederatedUpstream and FederatedVirtualService. The federated versions target one or more clusters and a namespace within each cluster.

In the demo environment two Kubernetes services have been deployed, echo-blue in the local cluster and echo-green in the remote cluster. A FederatedUpstream resource has been created for the echo-blue service on the local cluster. We can view the FederatedUpstream by running the following:

```
kubectl get FederatedUpstream -n gloo-system
```

```
NAME                   AGE
default-service-blue   13m
```

There will be a matching Upstream for the FederatedUpstream in each cluster specified by the Custom Resource. We can see the matching Upstream in the local cluster by running the following:

```
kubectl get Upstream -n gloo-system default-service-blue-10000
```

```
NAME                         AGE
default-service-blue-10000   18m
```

The FederatedUpstream is associated with a FederatedVirtualService that provides a simple route to the Upstream. We can view the FederatedVirtualService by running the following:

```
kubectl get FederatedVirtualService -n gloo-system
```

```
NAME           AGE
simple-route   16m
```

Just like the FederatedUpstream, the FederatedVirtualService will create a VirtualService in each targeted cluster. We can view the VirtualService by running the following:

```
kubectl get VirtualService -n gloo-system
```

```
NAME           AGE
simple-route   10m
```

We will use these federated resources as part of the service failover configuration.

### Service failover

When an Upstream fails or becomes unhealthy, Gloo Edge Federation can automatically shift traffic over to a different Gloo Edge instance and Upstream. The demo environment has two Kubernetes services, one running in the default namespace of each cluster. The echo-blue service is running in the local cluster and the echo-green service is running in the remote cluster. 

We can create a FailoverScheme in Gloo Edge Federation that specifies the echo-blue service as the primary and echo-green as a failover target. There can be multiple failover targets in different clusters and namespaces with different priorities.

We can view the FailoverScheme by running the following:

```
kubectl get FailoverScheme -n gloo-system
```

```
NAME                   AGE
failover-test-scheme   21m
```

There's a bit more to the setup, which you can read about it in the [Service Failover guide]({{% versioned_link_path fromRoot="/guides/gloo_federation/service_failover/" %}}).

We can try out the service failover by first trying to contact the echo-blue service, then forcing a failure, and validating the echo-green service takes over. You will need two terminals running for this. The first terminal will run port forward commands and the second will interact with the services.

```
# Curl the route to reach the blue pod. You should see a return value of "blue-pod".

## First terminal
kubectl port-forward -n gloo-system svc/gateway-proxy 8080:80

## Second terminal
curl localhost:8080/

# Force the health check to fail

## First terminal
kubectl port-forward deploy/echo-blue-deployment 19000

## Second terminal
curl -X POST  localhost:19000/healthcheck/fail

# See that the green pod is now being reached, with the curl command returning "green-pod".

## First terminal
kubectl port-forward -n gloo-system svc/gateway-proxy 8080:80

## Second terminal
curl localhost:8080/
```

## Cleanup

When you are finished working with the demo environment, you can delete the resources by simply deleting the two kind clusters:

```
kind delete cluster --name local
kind delete cluster --name remote
```

## Next Steps

Now that you've had a chance to investigate some of the features of Gloo Edge Federation, now might be a good time to read a bit more about the [concepts]({{% versioned_link_path fromRoot="/introduction/gloo_federation/" %}}) behind Gloo Edge Federation or you can try [installing]({{% versioned_link_path fromRoot="/installation/gloo_federation/" %}}) it in your own environment.
