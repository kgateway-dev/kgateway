---
title: Federated Configuration
description: Setting up federated configuration
weight: 25
---

Gloo Edge Federation enables you to create consistent configurations across multiple Gloo Edge instances. The resources being configured could be resources such as Upstreams, UpstreamGroups, and Virtual Services. In this guide you will learn how to add a Federated Upstream and Virtual Service to a registered cluster being managed by Gloo Edge Federation.

## Prerequisites

To successfully follow this guide, you will need to have Gloo Edge Federation deployed on an admin cluster and a registered cluster to target for configuration. We recommend that you follow the Gloo Edge Federation [installation guide]({{% versioned_link_path fromRoot="/installation/gloo_federation/" %}})  and [Cluster Registration guide]({{% versioned_link_path fromRoot="/guides/gloo_federation/cluster_registration/" %}}) to prepare for this guide if you haven’t already done so.

## Create the Federated Resources

We are going to create a Federated Upstream and Federated Virtual Service. We can do this by using kubectl to create the necessary Custom Resources. Once the CR is created, the Gloo Edge Federation controller will create the necessary resources on any designated clusters under the configured namespace.

In this example, we will be using the admin cluster where Gloo Edge Federation is running. You can select a different cluster by changing the placement values. Our registered cluster is named `local` and Gloo Edge Enterprise is using the default `gloo-system` namespace.

### Create the Federated Upstream

Let’s create the Federated Upstream by running the following command in the context of the admin cluster where Gloo Edge Federation is running:

```yaml
kubectl apply -f - <<EOF
apiVersion: fed.gloo.solo.io/v1
kind: FederatedUpstream
metadata:
  name: my-federated-upstream
  namespace: gloo-system
spec:
  placement:
    clusters:
      - local
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

As you can see in the spec for the FederatedUpstream resource, the placement settings specify that the Upstream should be created in the `local` cluster in the `gloo-system` namespace. The template settings define the properties of the Upstream being created.

Once we run the command, we can validate that it was successful by running the following:

```
kubectl get federatedupstreams -n gloo-system -oyaml
```

In the resulting output you should see the state as `PLACED` in the status section:

```yaml
  status:
    placementStatus:
      clusters:
        local:
          namespaces:
            gloo-system:
              state: PLACED
      observedGeneration: "1"
      state: PLACED
      writtenBy: gloo-fed-5dd98c7bfd-96sn2
```

Looking at the Upstream resources in the local cluster, we can confirm the Upstream has been created:

```
kubectl get upstream -n gloo-system fed-upstream
```

```
NAME              AGE
fed-upstream      97m
```

Now we can create a Virtual Service for the Upstream.

### Create a Federated Virtual Service

Let’s create a Virtual Service that exposes the Upstream from the previous step. We will run the following command in the context of the admin cluster where Gloo Edge Federation is running:

```yaml
kubectl apply -f - <<EOF
apiVersion: fed.gateway.solo.io/v1
kind: FederatedVirtualService
metadata:
  name: my-federated-vs
  namespace: gloo-system
spec:
  placement:
    clusters:
      - local
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
      name: fed-virtualservice
EOF
```

Once we run the command, we can validate that it was successful by running the following:

```
kubectl get federatedvirtualservice -n gloo-system -oyaml
```

In the resulting output you should see the state as `PLACED` in the status section:

```yaml
  status:
    placementStatus:
      clusters:
        local:
          namespaces:
            gloo-system:
              state: PLACED
      observedGeneration: "1"
      state: PLACED
      writtenBy: gloo-fed-5dd98c7bfd-96sn2
```

Looking at the Virtual Service resources in the local cluster, we can confirm the Virtual Service has been created:

```
kubectl get virtualservice -n gloo-system fed-virtualservice
```

```
NAME                 AGE
fed-virtualservice   4m39s
```

Any updates made to the Federated Upstream or Federated Virtual Service will be applied to all clusters associated with the Custom Resource.

## Next Steps

Setting up Federated Configuration also enables Service Failover. You can check out the [guide for Service Failover]({{% versioned_link_path fromRoot="/guides/gloo_federation/service_failover/" %}}) next, or learn more about the [concepts]({{% versioned_link_path fromRoot="/introduction/gloo_federation/" %}}) behind Gloo Edge Federation.