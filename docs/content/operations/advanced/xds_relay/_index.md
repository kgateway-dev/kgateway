
---
title: "Beta: xDS Relay"
description: This document explains how to use a compressed spec for the Proxy CRD.
weight: 30
---

{{% notice warning %}}
xDS relay is currently aviailable in Gloo Edge 1.11.x and later as a beta feature. Do not use this feature in production environments.
{{% /notice %}}

{{% notice warning %}}
This feature is not supported for the following non-default installation modes of Gloo Edge: REST Endpoint Discovery (EDS), Gloo Edge mTLS mode, Gloo Edge with Istio mTLS mode
{{% /notice %}}

To protect against control plane downtime, you can install Gloo Edge alongside the `xds-relay` Helm chart. This Helm chart installs a deployment of `xds-relay` pods that serve as intermediaries between Envoy proxies and the xDS server of Gloo Edge.

The presence of `xds-relay` intermediary pods serve two purposes. First, it separates the lifecycle of Gloo Edge from the xDS cache proxies. For example, a failure during a Helm upgrade will not cause the loss of the last valid xDS state. Second, it allows you to scale `xds-relay` to as many replicas as needed, since Gloo Edge uses only one replica. Without `xds-relay`, a failure of the single Gloo Edge replica causes any new Envoy proxies to be created without a valid configuration.


To enable:

Install the xds-relay Helm chart, which supports version 2 and 3 of the Envoy API:
  - `helm repo add xds-relay https://storage.googleapis.com/xds-relay-helm`
  - `helm install xdsrelay xds-relay/xds-relay`

If needed, change the default values for the xds-relay chart:
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

- Install Gloo Edge with the following helm values for each proxy (envoy) to point them towards xds-relay:
```yaml
gatewayProxies:
  gatewayProxy: # do the following for each gateway proxy
    xdsServiceAddress: xds-relay.default.svc.cluster.local
    xdsServicePort: 9991
```