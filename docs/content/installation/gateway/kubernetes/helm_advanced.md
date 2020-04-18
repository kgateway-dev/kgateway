---
title: "Last mile helm chart customization with Helm and Kustomize"
menuTitle: "Kubernetes"
description: How to make tweaks to the existing Gloo helm chart.
weight: 20
---


Helm 3.1 supports the notion of a post render step, that allows customizing a chart,
without needed to modify the chart itself.

In this example, we will add a sysctl value to the gateway-proxy pod.

We are going to:
1. Create customization file
1. Create a patch to add our desired sysctl
1. Demonstrate that it was applied correctly using `helm template`


### Create Kustomization

First, lets create the patch we want to apply. This patch will be merged to our existing
objects, so it looks very similar to a regular deployment definition. We add a `securityContext` to
the pod with out new sysctl value:

```bash
cat > sysctl-patch.yaml <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway-proxy
  namespace: gloo-system
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


### Test

We can render our chart using helm template and see our changes in it:

```bash
helm template gloo/gloo --post-renderer ./kustomize.sh
```

In the output you will see our newly added sysctl:
```
        - mountPath: /etc/envoy
          name: envoy-config
      securityContext:
        sysctls:
        - name: net.netfilter.nf_conntrack_tcp_timeout_close_wait
          value: "10"

```