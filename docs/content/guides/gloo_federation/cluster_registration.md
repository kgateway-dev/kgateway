---
title: Cluster Registration
description: Registering a cluster with Gloo Edge Federation
weight: 20
---

Gloo Edge Federation monitors clusters that have been registered using `glooctl` and automatically discovers instances of Gloo Edge deployed on said clusters. Once the registration process is complete, Gloo Edge Federation can create federated configuration resources and apply them to Gloo Edge instances running in registered clusters.

In this guide, we will walk through the process of registering a Kubernetes cluster with Gloo Edge Federation.

![]({{% versioned_link_path fromRoot="/img/gloo-fed-arch-cluster-reg.png" %}})

## Prerequisites

To successfully follow this guide, you will need to have Gloo Edge Federation deployed on an admin cluster and a cluster to use for registration. The cluster can either be the admin cluster or a remote cluster. We recommend that you follow the Gloo Edge Federation [installation guide]({{% versioned_link_path fromRoot="/guides/gloo_federation/installation/" %}}) to prepare for this guide.

## Register a cluster

Gloo Edge Federation will not automatically register the Kubernetes cluster it is running on. Both the local cluster and any remote clusters must be registered manually. The registration process will create a service account, cluster role, and cluster role binding on the target cluster, and store the access credentials in a Kubernetes secret resource in the admin cluster.

### Registration with glooctl

For our example we will be using the admin cluster for registration. The name of the kubectl context associated with that cluster is `gloo-fed`. We will give this cluster the name `local` for Gloo Edge Federation to refer to it.

The registration is performed by running the following command:

```
glooctl cluster register --cluster-name local --remote-context gloo-fed
# --cluster-name will be the name given to the Gloo Fed resource representing the target cluster
# --remote-context is the name of the target Kubernetes context as shown in your ~/kube/config file
```

{{< notice note >}}
If you are running the registration command against a kind cluster on MacOS or Linux, you will need to append the `local-cluster-domain-override` flag to the command:

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

Credentials for the remote cluster are stored in a secret in the `gloo-system` namespace. The secret name will be the same as the `cluster-name` specified when registering the cluster.

```
kubectl get secret -n gloo-system local
```

```
NAME    TYPE                 DATA   AGE
local   solo.io/kubeconfig   1      37s
```

In the registered cluster, Gloo Edge Federation has created a service account, cluster role, and role binding. They can be viewed by running the following commands:

```
kubectl --cluster kind-local get serviceaccount local -n gloo-system
kubectl --cluster kind-local get clusterrole gloo-federation-controller
kubectl --cluster kind-local get clusterrolebinding local-gloo-federation-controller-clusterrole-binding
```

Once a cluster has been registered, Gloo Edge Federation will automatically discover all instances of Gloo Edge within the cluster. The discovered instances are stored in a Custom Resource of type `GlooInstance` in the `gloo-system` namespace. You can view the discovered instances by running the following:

```
kubectl get glooinstances -n gloo-system
```

```
NAME                      AGE
local-gloo-system         95m
```

You have now successfully added a (local or remote) cluster to Gloo Edge Federation. You can repeat the same process for any other clusters you want to include in Gloo Edge Federation.


### Manual registration

Below is a list of the Kubernetes resources that are created upon cluster registration with `glooctl`:

On the remote cluster:
- creates a **ServiceAccount** named as the "cluster-name" argument
- Kubernetes will generate a **Secret** with a token for this **ServiceAccount**
- creates a **ClusterRole** named `gloo-federation-controller` that will be used by this SA to manage Gloo resources on this remote cluster
- creates a **ClusterRoleBinding** to associate the **ServiceAccount** and the **ClusterRole**

On the cluster running Gloo Federation:
- creates a **KubernetesCluster** CR named as the "cluster-name" argument
- creates a **Secret** with the `kubeconfig` of the remote cluster, with the token of the **ServiceAccount** created on that cluster (secret type is `solo.io/kubeconfig`)
- creates a **GlooInstance** CR for each discovered Gloo Edge deployment (Gloo Fed will look for )



If you cannot use the `glooctl` CLI, you can simulate its behaviour with the following steps.

{{< notice note >}}
If the cluster to register is running with KinD, you may want to empty the ca-cert section of your `~/kube/config` file, and set `insecure-skip-tls-verify: true`
{{< /notice >}}

1. The rest of this guide assumes that the target cluster being registered is exported as shown below:
    ```shell
    export CLUSTER_NAME=target-cluster
    ```

2. **[OPTIONAL]** If Gloo Edge is not already running on the remote cluster that is being registered, consider the following extra steps:
    ```shell
    # install the Gloo Federation CRDs on the target cluster:
    helm fetch glooe/gloo-ee --version ${GLOO_VERSION} --devel --untar --untardir /tmp/glooee-${GLOO_VERSION}
    kubectl --context ${CLUSTER_NAME} apply -f /tmp/glooee-${GLOO_VERSION}/gloo-ee/charts/gloo-fed/crds/
   
    # install the Gloo Edge CRDs on the admin cluster:
    kubectl apply -f /tmp/glooee-${GLOO_VERSION}/gloo-ee/charts/gloo/crds/
    ```

3. on the remote cluster, create the following Kubernetes resources:
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

4. prepare a kube config file like the following. Mind the `server` field which is the address used by Gloo Federation to connect to that remote cluster API-server:
    ```shell
    cat > kc.yaml <<EOF
    apiVersion: v1
    clusters:
    - cluster:
        certificate-authority-data: "" # DO NOT empty the value here unless you are using KinD
        server: https://host.docker.internal:65404 # put here the api-server address of the target cluster
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

5. on the cluster running gloo-fed, create the following Kubernetes resources:
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

6. at this point, Gloo Federation will look for Gloo Edge deployments in the target cluster. Once discovered, Gloo Federation will create a new `GlooInstance` CR in the admin cluster. 



For each cluster, you should have a new `KubernetesCluster` CR in the `gloo-system` namespace.

For each Gloo Edge deployment, you should find a new `GlooInstance` CR in the `gloo-system` namespace.


## Next Steps

With a registered cluster in Gloo Edge Federation, now might be a good time to read a bit more about the [concepts]({{% versioned_link_path fromRoot="/introduction/gloo_federation/" %}}) behind Gloo Edge Federation or you can try out [Federated Configuration]({{% versioned_link_path fromRoot="/guides/gloo_federation/federated_configuration/" %}}) feature.