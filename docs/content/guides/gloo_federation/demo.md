---
title: Quick demo
description: Run a quick demo of Gloo Federation in a local environment
weight: 10
---

With Gloo Edge Federation, you can manage multiple Gloo Edge instances in multiple Kubernetes clusters. In this guide, you use `glooctl` to create a demonstration environment that federates Gloo Edge across clusters.

## Before you begin

Make sure that you have the following tools installed.

* **Docker** - Runs the containers for Kind and all pods inside the clusters.
* **Kubectl** - Executes commands against the Kubernetes clusters.
* **[Kind](https://kind.sigs.k8s.io/)** - Deploys two Kubernetes clusters by using containers that run on Docker.
* **Helm** - Deploys the Gloo Edge Federation and Gloo Edge charts.
* **Glooctl** - Sets up the demonstration environment.
* **Gloo Edge Enterprise license** - You need a license key to deploy the demonstration environment. To request a license, [contact Sales](https://www.solo.io/company/contact/).


## Deploy the demonstration environment

Use the `glooctl demo` to set up the environment. The end result is a fully functioning local environment that runs two Kubernetes clusters, Gloo Edge Enterprise, and Gloo Edge Federation. 

You can generate the demo environment by running the following command:

```
glooctl demo federation --license-key <license key>
```

That command performs the following actions: 

1. Deploys two kind clusters called local and remote
2. Installs Gloo Edge Enterprise on both clusters in the `gloo-system` namespace
3. Installs Gloo Edge Federation on the local cluster in the `gloo-system` namespace
4. Registers both Gloo Edge Enterprise instances with Gloo Edge Federation
5. Federates configuration resources
6. Creates a Failover Service configuration using both Gloo Edge Enterprise instances

After the demo environment provisions, explore the environment in the following sections.

## Explore the demo environment

The local demo environment is a sandbox for you to explore the functionality of Gloo Edge Federation. Let's take a look at what is deployed.

### Kubernetes clusters and Gloo Edge installations

1. View the local and remote kind clusters.

   ```
   kind get clusters
   ```
   Example output:
   ```
   local
   remote
   ```
2. Verify that you have `kubectl` config contexts for `kind-local` and `kind-remote`. By default, your `kubectl` context is set to `kind-local` for the local cluster.
   ```
   kubectl config get-contexts 
   ```
   Example output:
   ```
   CURRENT   NAME        CLUSTER     AUTHINFO                               
   *         kind-local  kind-local  kind-local
             kind-remote kind-remote kind-remote  
   ```
3. Verify that the Gloo Edge components are running on each cluster.

   ```
   kubectl get deployment -l app=gloo -n gloo-system --context kind-local
   kubectl get deployment -l app=gloo -n gloo-system --context kind-remote
   ```
   Example output:
   ```
   NAME            READY   UP-TO-DATE   AVAILABLE   AGE
   discovery       1/1     1            1           18m
   extauth         1/1     1            1           18m
   gateway         1/1     1            1           18m
   gateway-proxy   1/1     1            1           18m
   gloo            1/1     1            1           18m
   observability   1/1     1            1           18m
   rate-limit      1/1     1            1           18m
   redis           1/1     1            1           18m
   NAME            READY   UP-TO-DATE   AVAILABLE   AGE
   discovery       1/1     1            1           17m
   gateway         1/1     1            1           17m
   gateway-proxy   1/1     1            1           17m
   gloo            1/1     1            1           17m
   ```
4. Verify the Gloo Edge Federation installation is running in the local cluster.

   ```
   kubectl get deployment -l app=gloo-fed -n gloo-system --context kind-local
   ```
   Example output:
   ```
   NAME               READY   UP-TO-DATE   AVAILABLE   AGE
   gloo-fed           1/1     1            1           24m
   gloo-fed-console   1/1     1            1           24m
   ```

### Cluster registration

For Gloo Edge to be federated, each Kubernetes cluster that runs Gloo Edge Enterprise must be registered. After a cluster is registered, Gloo Edge Federation automatically discovers all instances of Gloo Edge on the cluster. The `glooctl federation demo` command registered the clusters for you. 

1. Verify that Gloo Edge Federation automatically discovered each instance of Gloo on the registered clusters. The discovered instances are stored in a Custom Resource of type `glooinstances.fed.solo.io` in the `gloo-system` namespace. The naming of each resource follows the convention `clustername-gloo-namespace`. 

   ```
   kubectl get glooinstances -n gloo-system --context kind-local
   ```
   Example output:
   ```
   NAME                      AGE
   kind-local-gloo-system    4m33s
   kind-remote-gloo-system   4m1s
   ```
2. Check that Gloo Edge Federation automatically created the necessary credentials for the remote cluster. These credentials include the service account, cluster role, and cluster role binding in the remote cluster.
   ```
   kubectl get serviceaccount kind-remote -n gloo-system --context kind-remote
   kubectl get clusterrole gloo-federation-controller --context kind-remote
   kubectl get clusterrolebinding kind-remote-gloo-federation-controller-clusterrole-binding --context kind-remote
   ```
3. Verify that the remote cluster credentials are stored as a secret in the local cluster. The secret name is the same as the `cluster-name` that was specified when registering the cluster.
   ```
   kubectl get secret -n gloo-system kind-remote --context kind-local
   ```
   Example output:
   ```
   NAME          TYPE                 DATA   AGE
   kind-remote   solo.io/kubeconfig   1      2m53s
   ```

### Federated configuration

Gloo Edge Federation lets you create consistent configurations across multiple Gloo Edge instances. You can configure Gloo resources such as Upstreams, UpstreamGroups, and Virtual Services. Then, Gloo creates federated versions with separate Custom Resource Definitions, like FederatedUpstream and FederatedVirtualService. The federated versions target one or more clusters and a namespace within each cluster.

In the demo environment, two Kubernetes apps are created:
* A federated `echo-blue` deployment and related services in the local cluster.
* An unfederated `echo-green` deployment and related services in the remote cluster.

Check the federated resources:
1. Check that the `default-service-blue` FederatedUpstream is created on the local cluster for the `echo-blue` deployment.
   ```
   kubectl get FederatedUpstream -n gloo-system
   ```
   Example output:
   ```
   NAME                   AGE
   default-service-blue   13m
   ```
2. Verify that a matching Upstream for the FederatedUpstream is created in each cluster. In this example, the matching Upstream is only in the local cluster.

   ```
   kubectl get Upstream -n gloo-system default-service-blue-10000
   ```
   Example output:
   ```
   NAME                         AGE
   default-service-blue-10000   18m
   ```
3. Verify that the `simple-route` FederatedVirtualService is created for the Upstream. 

   ```
   kubectl get FederatedVirtualService -n gloo-system
   ```
   Example output:
   ```
   NAME           AGE
   simple-route   16m
   ```
4. Verify that a matching VirtualService is created for the FederatedVirtualService in the cluster.

   ```
   kubectl get VirtualService -n gloo-system
   ```
   Example output:
   ```
   NAME           AGE
   simple-route   10m
   ```

You can use these federated resources to configure service failover across federated Gloo Edge instances.

### Service failover

When an Upstream fails or becomes unhealthy, Gloo Edge Federation can automatically shift traffic over to a different Gloo Edge instance and Upstream. The demo environment has two Kubernetes services, one running in the default namespace of each cluster. The `echo-blue` deployment is running in the local cluster and the `echo-green` deployment is running in the remote cluster. 

1. Review the FailoverScheme that configures the upstream for the `echo-blue` deployment as the primary service and the upstream for the `echo-green` deployment as a failover target. In the FailoverScheme, you can also configure multiple failover targets in different clusters and namespaces with different priorities. For more information, see the [Service Failover guide]({{% versioned_link_path fromRoot="/guides/gloo_federation/service_failover/" %}}).

   ```
   kubectl get FailoverScheme -n gloo-system -o yaml
   ```
   {{< highlight yaml "hl_lines=17-27" >}}
apiVersion: v1
items:
- apiVersion: fed.solo.io/v1
  kind: FailoverScheme
  metadata:
    annotations:
      kubectl.kubernetes.io/last-applied-configuration: |
        {"apiVersion":"fed.solo.io/v1","kind":"FailoverScheme","metadata":{"annotations":{},"name":"failover-test-scheme","namespace":"gloo-system"},"spec":{"failoverGroups":[{"priorityGroup":[{"cluster":"kind-remote","upstreams":[{"name":"default-service-green-10000","namespace":"gloo-system"}]}]}],"primary":{"clusterName":"kind-local","name":"default-service-blue-10000","namespace":"gloo-system"}}}
    creationTimestamp: "2022-11-29T16:37:40Z"
    finalizers:
    - fed.solo.io/finalizer
    generation: 1
    name: failover-test-scheme
    namespace: gloo-system
    resourceVersion: "19570"
    uid: c0b4a5fb-3b64-46a0-958f-f2ff035c50ed
  spec:
    failoverGroups:
    - priorityGroup:
      - cluster: kind-remote
        upstreams:
        - name: default-service-green-10000
          namespace: gloo-system
    primary:
      clusterName: kind-local
      name: default-service-blue-10000
      namespace: gloo-system
kind: List
metadata:
  resourceVersion: ""
   {{< /highlight >}}
1. Start the gateway service locally so that you can test failover across services.
   ```
   kubectl port-forward -n gloo-system svc/gateway-proxy 8080:80
   ```
2. In a new tab in your terminal, verify that you can send a request to the `echo-blue` deployment.
   ```
   curl localhost:8080/
   ```
   Example output:
   ```
   "blue-pod"
   ```
3. In a new tab in your terminal, start the `echo-blue` deployment.
   ```
   kubectl port-forward deploy/echo-blue-deployment 19000
   ```
4. In the previous tab in your terminal, update the `echo-blue` deployment to simulate a failure.
   ```
   curl -X POST  localhost:19000/healthcheck/fail
   ```
   Example output:
   ```
   OK
   ```
5. Repeat your previous request to contact the service. Instead of the blue pod, you see the green pod.
   ```
   curl localhost:8080/
   ```
   Example output:
   ```
   "green-pod"
   ```

Good job! You set up and verified failover across federated Gloo services.

## Cleanup

When you are finished working with the demo environment, you can delete the resources by simply deleting the two kind clusters:

```
kind delete cluster --name local
kind delete cluster --name remote
```

## Next Steps

Now that you've had a chance to investigate some of the features of Gloo Edge Federation, now might be a good time to read a bit more about the [concepts]({{% versioned_link_path fromRoot="/introduction/gloo_federation/" %}}) behind Gloo Edge Federation or you can try [installing]({{% versioned_link_path fromRoot="/installation/gloo_federation/" %}}) it in your own environment.