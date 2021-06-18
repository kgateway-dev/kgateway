---
title: Cluster Registration
description: Registering a cluster with Gloo Edge Federation
weight: 20
---

Gloo Edge Federation monitors clusters that have been registered using `glooctl` and automatically discovers instances of Gloo Edge deployed on said clusters. Once the registration process is complete, Gloo Edge Federation can create federated configuration resources and apply them to Gloo Edge instances running in registered clusters.

In this guide, we will walk through the process of registering a Kubernetes cluster with Gloo Edge Federation.

## Prerequisites

To successfully follow this guide, you will need to have Gloo Edge Federation deployed on an admin cluster and a cluster to use for registration. The cluster can either be the admin cluster or a remote cluster. We recommend that you follow the Gloo Edge Federation [installation guide]({{% versioned_link_path fromRoot="/installation/gloo_federation/" %}}) to prepare for this guide.

## Register remote cluster

Gloo Edge Federation will not automatically register the Kubernetes cluster it is running on. Both the local cluster and any remote clusters must be registered manually. The registration process will create a service account, cluster role, and cluster role binding on the target cluster, and store the access credentials in a Kubernetes secret resource in the admin cluster.

For our example we will be using the admin cluster for registration. The name of the kubectl context associated with that cluster is gloo-fed. We will give this cluster the name `local` for Gloo Edge Federation to refer to it.

The registration is performed by running the following command:

```
glooctl cluster register --cluster-name local --remote-context gloo-fed
```

{{< notice note >}}
If you are running the registration command against a kind cluster on MacOS or Linux, you will need to append the `local-cluster-domain-override` flag to the command:

<pre><code>
# MacOS
glooctl cluster register --cluster-name local --remote-context kind-local \
  --local-cluster-domain-override host.docker.internal:6443

</code></pre>


<pre><code>
# Linux
# Get the IP address of the local cluster control plane
LOCAL_IP=$(docker exec local-control-plane ip addr show dev eth0 | sed -nE 's|\s*inet\s+([0-9.]+).*|\1|p')
glooctl cluster register --cluster-name local --remote-context kind-local \
  --local-cluster-domain-override $LOCAL_IP:6443


</code></pre>
{{< /notice >}}

Cluster registration creates a KubernetesCluster CR that contains information about the cluster
that was just registered, including its credentials.

Credentials for the remote cluster are stored in a secret in the gloo-system namespace. The secret name will be the same as the `cluster-name` specified when registering the cluster.

```
kubectl get secret -n gloo-system local
```

```
NAME    TYPE                 DATA   AGE
local   solo.io/kubeconfig   1      37s
```

In the registered cluster, Gloo Edge Federation has created a service account, cluster role, and role binding. They can be viewed by running the following commands:

```
kubectl get serviceaccount local -n gloo-system
kubectl get clusterrole gloo-federation-controller
kubectl get clusterrolebinding local-gloo-federation-controller-clusterrole-binding
```

Once a cluster has been registered, Gloo Edge Federation will automatically discover all instances of Gloo Edge within the cluster. The discovered instances are stored in a Custom Resource of type `glooinstances.fed.solo.io` in the `gloo-system` namespace. You can view the discovered instances by running the following:

```
kubectl get glooinstances -n gloo-system
```

```
NAME                      AGE
local-gloo-system         95m
```

You have now successfully added a remote cluster to Gloo Edge Federation. You can repeat the same process for any other clusters you want to include in Gloo Edge Federation.

## Next Steps

With a registered cluster in Gloo Edge Federation, now might be a good time to read a bit more about the [concepts]({{% versioned_link_path fromRoot="/introduction/gloo_federation/" %}}) behind Gloo Edge Federation or you can try out [Federated Configuration]({{% versioned_link_path fromRoot="/guides/gloo_federation/federated_configuration/" %}}) feature.