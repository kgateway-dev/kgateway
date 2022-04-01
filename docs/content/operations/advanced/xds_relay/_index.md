
---
title: "Beta: xDS Relay"
description: This document explains how to use a compressed spec for the Proxy CRD.
weight: 30
---

{{% notice warning %}}
xDS relay is available in Gloo Edge 1.11.x and later as a beta feature. This feature is not supported for the following non-default installation modes of Gloo Edge: REST Endpoint Discovery (EDS), Gloo Edge mTLS mode, Gloo Edge with Istio mTLS mode.
{{% /notice %}}

To protect against control plane downtime, you can install Gloo Edge alongside the `xds-relay` Helm chart. This Helm chart installs a deployment of `xds-relay` pods that serve as intermediaries between Envoy proxies and the xDS server of Gloo Edge.

The presence of `xds-relay` intermediary pods serve two purposes. First, it separates the lifecycle of Gloo Edge from the xDS cache proxies. For example, a failure during a Helm upgrade will not cause the loss of the last valid xDS state. Second, it allows you to scale `xds-relay` to as many replicas as needed, since Gloo Edge uses only one replica. Without `xds-relay`, a failure of the single Gloo Edge replica causes any new Envoy proxies to be created without a valid configuration.

1. Install the `xds-relay` Helm chart, which supports version 2 and 3 of the Envoy API.
   ```shell
   helm repo add xds-relay https://storage.googleapis.com/xds-relay-helm
   helm repo upgrade
   helm install xdsrelay xds-relay/xds-relay
   ```

2. Optional: Modify the default values for the `xds-relay` chart, such as to add resource requests and limits.
```yaml
deployment:
  replicas: 3
  image:
    pullPolicy: IfNotPresent
    registry: gcr.io/gloo-edge
    repository: xds-relay
    tag: %version%
# might want to set resources for prod deploy, e.g.:
#  resources:
#    requests:
#      cpu: 125m
#      memory: 256Mi
service:
  port: 9991
bootstrap:
  cache:
    # zero means no limit
    ttl: 0s
    # zero means no limit
    maxEntries: 0
  originServer:
    address: gloo.gloo-system.svc.cluster.local
    port: 9977
    streamTimeout: 5s
  logging:
    level: INFO
# might want to add extra, non-default identifiers
#extraLabels:
#  k: v
#extraTemplateAnnotations:
#  k: v
```

3. [Install Gloo Edge]({{< versioned_link_path fromRoot="/installation/enterprise/" >}}) with the following Helm values to point each Envoy proxy (envoy) to `xds-relay`.
```yaml
gatewayProxies:
  gatewayProxy: # do the following for each gateway proxy
    xdsServiceAddress: xds-relay.default.svc.cluster.local
    xdsServicePort: 9991
```