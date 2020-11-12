---
title: "Last Mile Helm Chart Customization"
menuTitle: "Kubernetes"
description: How to make tweaks to the existing Gloo Edge Helm chart with Helm and Kustomize.
weight: 20
---

# Motivation

Gloo Edge's helm chart is very customizable, but does not contain every possible kubernetes value you may want to tweak. In this document we will demonstrate a method of tweaking the helm release, without the need to directly modify Gloo Edge's helm chart.

This allows you to tailor the installation manifests to **your specific needs** quickly and easily.

We will use Helm 3.1 supports for [post rendering](https://helm.sh/docs/topics/advanced/#post-rendering). This allows us to tweak the rendered manifests just before they are applied to the cluster, without needed to modify the chart itself.

In this example, we will add a `sysctl` value to the Gloo Edge's `gateway-proxy` pod. We are going to:

1. Create customization file
1. Create a patch to add our desired sysctl
1. Demonstrate that it was applied correctly using `helm template`

# Prerequisites

To complete this you will need:

- kubectl ≥ 1.14 OR kustomize
- helm ≥ 3.1


# Create Kustomization

First, lets create the patch we want to apply. This patch will be merged to our existing
objects, so it looks very similar to a regular deployment definition. We add a `securityContext` to
the pod with out new sysctl value:

```bash
cat > sysctl-patch.yaml <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway-proxy
spec:
  template:
    spec:
      securityContext:
          sysctls:
          - name: net.netfilter.nf_conntrack_tcp_timeout_close_wait
            value: "10"
EOF
```

Helm post render works with stdin/stdout, and kustomize works with files. Let's bridge that gap
with a shell script:

```bash
cat > kustomize.sh <<EOF
#!/bin/sh
cat > base.yaml
# you can also use "kustomize build ." if you have it installed.
exec kubectl kustomize
EOF
chmod +x ./kustomize.sh
```

Finally, lets create our `kustomization.yaml`

```bash
cat > kustomization.yaml <<EOF
resources:
- base.yaml
patchesStrategicMerge:
- sysctl-patch.yaml
EOF
```


# Test

## Add the Helm repository for Gloo Edge

```bash
helm repo add gloo https://storage.googleapis.com/solo-public-helm
helm repo update
```

## Render
We can render our chart using helm template and see our changes in it:

```bash
helm template gloo/gloo --post-renderer ./kustomize.sh
```

In the output you will see our newly added sysctl:
```yaml
…
        - mountPath: /etc/envoy
          name: envoy-config
      securityContext:
        sysctls:
        - name: net.netfilter.nf_conntrack_tcp_timeout_close_wait
          value: "10"
…
```

## Apply

You can use this command to install \ upgrade your release:

```bash
kubectl create ns gloo-system
helm upgrade -i gloo gloo/gloo --namespace gloo-system --post-renderer ./kustomize.sh
```

Examine the `gateway-proxy` deployment, you will see the new value:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  …
  name: gateway-proxy
  namespace: gloo-system
spec:
  template:
    metadata:
      …
    spec:
      securityContext:
        sysctls:
        - name: net.netfilter.nf_conntrack_tcp_timeout_close_wait
          value: "10"
…
```