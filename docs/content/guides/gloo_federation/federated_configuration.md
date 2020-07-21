---
title: Federated Configuration
description: Setting up federated configuration
weight: 20
---

Gloo Federation enables you to create consistent configurations across multiple Gloo instances. The resources being configured could be resources such as Upstreams, UpstreamGroups and, Virtual Services. In this guide you will learn how to add a Federated Upstream and Virtual Service to a remote cluster being managed by Gloo Federation.

## Prerequisites

To successfully follow this guide, you will need to have Gloo Federation deployed on an administrative cluster and a remote cluster to target for configuration. We recommend that you follow the Getting Started guide to prepare for this guide if you haven’t already done so.

## Create the Federated Resources

We are going to create a Federated Upstream and Federated Virtual Service. We can do this be using kubectl to create the necessary Custom Resources. Once the CR is created, the Gloo Federation controller will create the necessary resources in the remote cluster under the configured namespace.

### Create the Federated Upstream

Let’s create the Federated Upstream by running the following command in the context of the administrative cluster where Gloo Federation is running:

```yaml
kubectl apply -f - <<EOF
apiVersion: fed.gloo.solo.io/v1
kind: FederatedUpstream
metadata:
  name: my-federated-upstream
  namespace: gloo-fed
spec:
  placement:
    clusters:
      - kind-remote
    namespaces:
      - gloo-system
  template:
    spec:
      static:
        hosts:
          - addr: solo.io
            port: 80
    metadata:
      name: fed-upstream
EOF
```

As you can see in the spec for the FederatedUpstream resource, the placement settings specify that the Upstream should be created in the kind-remote cluster in the gloo-system namespace. The template settings define the properties of the Upstream being created.

Once we run the command, we can validate that it was successful by running the following:

```
kubectl get federatedupstreams -n gloo-fed -oyaml
```

In the resulting output you should see the following in the status section:

```yaml
  status:
    placementStatus:
      clusters:
        remote:
          namespaces:
            gloo-system:
              state: PLACED
      observedGeneration: "2"
      state: PLACED
      writtenBy: gloo-fed-956f66f75-mwfrb
```

Looking at the Upstream resources in the remote cluster, we can confirm the Upstream has been created:

```
kubectl get upstream -n gloo-system fed-upstream --context kind-remote
```

```
NAME              AGE
fed-upstream   97m
```

Now we can created a Virtual Service for the Upstream.

### Create a Federated Virtual Service

Let’s create a Virtual Service that exposes the Upstream from the previous step. We will run the following command in the context of the administrative cluster where Gloo Federation is running:

```yaml
kubectl apply -f - <<EOF
apiVersion: fed.gateway.solo.io/v1
kind: FederatedVirtualService
metadata:
  name: my-federated-vs
  namespace: gloo-fed
spec:
  placement:
    clusters:
      - kind-remote
    namespaces:
      - gloo-system
  template:
    spec:
      virtualHost:
        domains:
          - "*"
        routes:
          - matchers:
              - exact: /solo
            options:
              prefixRewrite: /
            routeAction:
              single:
                upstream:
                  name: fed-upstream
                  namespace: gloo-system
    metadata:
      name: fed-upstream
EOF
```
