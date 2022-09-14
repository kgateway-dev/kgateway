---
title: Cluster Registration
description: Registering a cluster with Gloo Edge Federation
weight: 20
---

Gloo Edge Federation monitors and automatically discovers instances of Gloo Edge on clusters that are registered with `glooctl`. After the registration process is complete, you can use Gloo Edge Federation to create and apply federated configuration resources across registered clusters.

In this guide, you register a Kubernetes cluster with Gloo Edge Federation.

![Figure of federated architecture]({{% versioned_link_path fromRoot="/img/gloo-fed-arch-cluster-reg.png" %}})

## Prerequisites

To successfully follow this guide, you need to have Gloo Edge Federation deployed on an admin cluster and a cluster to use for registration. The cluster can either be the admin cluster or a remote cluster. For instructions, follow the Gloo Edge Federation [installation guide]({{% versioned_link_path fromRoot="/guides/gloo_federation/installation/" %}}).

## Register a cluster

Gloo Edge Federation does not automatically register the Kubernetes cluster that it runs on. You must register both the local cluster and any remote clusters. The registration process creates a service account, cluster role, and cluster role binding on the target cluster, and stores the access credentials in a Kubernetes secret resource in the admin cluster.

### Registration with glooctl

To register a cluster with `glooctl`, run the following command. This example registers the admin cluster.

* `--cluster-name`: The name for the Gloo Fed resource that represents the target cluster you want to register. In this example, the cluster name is `local`.
* `--remote-context`: The name of the target cluster's Kubernetes context as shown in your `~/kube/config` file. In this example, the context for the cluster is `gloo-fed`.

```
glooctl cluster register --cluster-name local --remote-context gloo-fed
```

{{< notice note >}}
If you are running the registration command for a kind cluster on MacOS or Linux, append the `local-cluster-domain-override` flag to the command:

<pre><code># MacOS
glooctl cluster register --cluster-name local --remote-context kind-local \
  --local-cluster-domain-override host.docker.internal[:6443]
</code></pre>


<pre><code># Linux
# Get the IP address of the local cluster control plane
LOCAL_IP=$(docker exec local-control-plane ip addr show dev eth0 | sed -nE 's|\s*inet\s+([0-9.]+).*|\1|p')
glooctl cluster register --cluster-name local --remote-context kind-local \
  --local-cluster-domain-override $LOCAL_IP:6443
</code></pre>
{{< /notice >}}

Cluster registration creates a **KubernetesCluster** CR that contains information about the cluster
that was just registered, including its credentials.

Credentials for the remote cluster are stored in a secret in the `gloo-system` namespace. The secret name is the same as the `cluster-name` that you specified during registration.

```
kubectl get secret -n gloo-system local
```

```
NAME    TYPE                 DATA   AGE
local   solo.io/kubeconfig   1      37s
```

In the registered cluster, Gloo Edge Federation has created a Kubernetes service account, cluster role, and role binding. You can review these resources by running the following commands:

```
kubectl --cluster kind-local get serviceaccount local -n gloo-system
kubectl --cluster kind-local get clusterrole gloo-federation-controller
kubectl --cluster kind-local get clusterrolebinding local-gloo-federation-controller-clusterrole-binding
```

After a cluster is registered, Gloo Edge Federation automatically discovers all instances of Gloo Edge within the cluster. The discovered instances are stored in a Custom Resource of type `GlooInstance` in the `gloo-system` namespace. You can view the discovered instances by running the following command:

```
kubectl get glooinstances -n gloo-system
```

```
NAME                      AGE
local-gloo-system         95m
```

You have now successfully added a local or remote cluster to Gloo Edge Federation. You can repeat the same process for any other clusters you want to include in Gloo Edge Federation.


### Manual registration

When you register a cluster with `glooctl`, the following Kubernetes resources are created in your cluster.

On the remote cluster:
- A **ServiceAccount** with the same name as the `cluster-name` argument in the `glooctl` command.
- A **Secret** with a token for this **ServiceAccount**.
- A **ClusterRole** named `gloo-federation-controller` that the service account uses to manage Gloo resources on this remote cluster.
- A **ClusterRoleBinding** to associate the **ServiceAccount** and the **ClusterRole**.

On the local cluster that runs Gloo Federation:
- A **KubernetesCluster** custom resource with the same name as the `cluster-name` argument in the `glooctl` command.
- A **Secret** with the `kubeconfig` and token of the **ServiceAccount** created on the remote cluster. The secret type is `solo.io/kubeconfig`.
- A **GlooInstance** custom resource for each Gloo Edge deployment that Gloo Fed discovers.

If you cannot use the `glooctl` CLI, you can manually create these resources with the following steps.

{{< notice note >}}
If the cluster to register is running with KinD, empty the ca-cert section of your `~/kube/config` file, and set `insecure-skip-tls-verify: true`.
{{< /notice >}}

1. Set the cluster name that you want to register as an environment variable that is used in the rest of this guide.
    ```shell
    export CLUSTER_NAME=target-cluster
    ```

2. **Optional**: If Gloo Edge is not already running on the remote cluster, [install Gloo Edge]({{% versioned_link_path fromRoot="/guides/gloo_federation/installation/" %}}), such as with the following steps.
    ```shell
    # install the Gloo Federation CRDs on the target cluster:
    helm fetch glooe/gloo-ee --version ${GLOO_VERSION} --devel --untar --untardir /tmp/glooee-${GLOO_VERSION}
    kubectl --context ${CLUSTER_NAME} apply -f /tmp/glooee-${GLOO_VERSION}/gloo-ee/charts/gloo-fed/crds/
   
    # install the Gloo Edge CRDs on the admin cluster:
    kubectl apply -f /tmp/glooee-${GLOO_VERSION}/gloo-ee/charts/gloo/crds/
    ```

3. On the remote cluster, create the following Kubernetes resources.
    ```shell
    kubectl --context ${CLUSTER_NAME} create ns gloo-system
    kubectl --context ${CLUSTER_NAME} -n gloo-system create sa ${CLUSTER_NAME}
    secret=$(kubectl --context ${CLUSTER_NAME} -n gloo-system get sa ${CLUSTER_NAME} -o jsonpath="{.secrets[0].name}")
    token=$(kubectl --context ${CLUSTER_NAME} -n gloo-system get secret $secret -o jsonpath="{.data.token}" | base64 -d)
    
    kubectl --context ${CLUSTER_NAME} -n gloo-system apply -f - <<EOF
    apiVersion: rbac.authorization.k8s.io/v1
    kind: ClusterRole
    metadata:
      name: gloo-federation-controller
    rules:
    - apiGroups:
      - gloo.solo.io
      - gateway.solo.io
      - enterprise.gloo.solo.io
      - ratelimit.solo.io
      - graphql.gloo.solo.io
      resources:
      - '*'
      verbs:
      - '*'
    - apiGroups:
      - apps
      resources:
      - deployments
      - daemonsets
      verbs:
      - get
      - list
      - watch
    - apiGroups:
      - ""
      resources:
      - pods
      - nodes
      - services
      verbs:
      - get
      - list
      - watch
    ---
    apiVersion: rbac.authorization.k8s.io/v1
    kind: ClusterRoleBinding
    metadata:
      name: ${CLUSTER_NAME}-gloo-federation-controller-clusterrole-binding
    roleRef:
      apiGroup: rbac.authorization.k8s.io
      kind: ClusterRole
      name: gloo-federation-controller
    subjects:
    - kind: ServiceAccount
      name: ${CLUSTER_NAME}
      namespace: gloo-system
    EOF
    ```

4. Prepare a Kubernetes config file. Note that the `server` field must match the address for Gloo Federation to connect to that remote cluster's API server.
    ```shell
    cat > kc.yaml <<EOF
    apiVersion: v1
    clusters:
    - cluster:
        certificate-authority-data: "" # DO NOT empty the value here unless you are using KinD
        server: https://host.docker.internal:65404 # enter the api-server address of the target cluster
        insecure-skip-tls-verify: true # DO NOT use this unless you know what you are doing
      name: ${CLUSTER_NAME}
    contexts:
    - context:
        cluster: ${CLUSTER_NAME}
        user: ${CLUSTER_NAME}
      name: ${CLUSTER_NAME}
    current-context: ${CLUSTER_NAME}
    kind: Config
    preferences: {}
    users:
    - name: ${CLUSTER_NAME}
      user:
        token: $token
    EOF
    
    kc=$(cat kc.yaml | base64 -w0)
    ```

5. On the cluster that runs `gloo-fed`, create the following Kubernetes resources.
    ```shell
    kubectl --context gloo-fed-cluster apply -f - <<EOF
    ---
    apiVersion: multicluster.solo.io/v1alpha1
    kind: KubernetesCluster
    metadata:
      name: ${CLUSTER_NAME}
      namespace: gloo-system
    spec:
      clusterDomain: cluster.local
      secretName: ${CLUSTER_NAME}
    ---
    apiVersion: v1
    kind: Secret
    metadata:
      name: ${CLUSTER_NAME}
      namespace: gloo-system
    type: solo.io/kubeconfig
    data:
      kubeconfig: $kc
    EOF
    ```

Now that the custom resources are created, Gloo Federation automatically discovers the Gloo Edge deployments in the target cluster.
* For each registered cluster, the local cluster that runs `gloo-fed` has a new `KubernetesCluster` CR in the `gloo-system` namespace.
* For each Gloo Edge deployment, the local cluster that runs `gloo-fed` has a new `GlooInstance` CR in the `gloo-system` namespace.

## Next Steps

With a registered cluster in Gloo Edge Federation, now might be a good time to read a bit more about the [concepts]({{% versioned_link_path fromRoot="/introduction/gloo_federation/" %}}) behind Gloo Edge Federation, or you can try out the [Federated Configuration]({{% versioned_link_path fromRoot="/guides/gloo_federation/federated_configuration/" %}}) feature.
