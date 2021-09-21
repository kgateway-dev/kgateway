---
title: Canary Upgrade (1.9.0+)
weight: 5
description: Upgrading Gloo Edge with a canary workflow
---

In this guide we will describe the necessary steps to upgrade your Gloo Edge or Gloo Edge Enterprise deployments using
a canary model. This guide assumes the older Gloo Edge version is at least 1.9.0.

{{% notice note %}}

In versions prior to 1.9.0, status reporting on Gloo CRs was not per namespace, thus we had little insight into the
state of a resource from the perspective of each canary installation. Further, the helm value overrides for the xds
service address and xds service port had not yet been added, e.g.:

```yaml
gatewayProxies:
  gatewayProxy: # do the following for each gateway proxy
    xdsServiceAddress: xds-relay.default.svc.cluster.local
    xdsServicePort: 9991
```

In versions prior to 1.8.0, there was a [bug](https://github.com/solo-io/gloo/issues/5030) that prevented canary deploys
from working as the old control plane would crash loop when it saw newly added fields.

{{% /notice %}}

##### Prereqs

Gloo Edge 1.9.0 or later installed.

##### Simple Canary (Recommended)

- **Install**
  - Install Gloo Edge with the new version to another namespace, e.g. `glooctl install gateway --version 1.9.0 -n gloo-system-canary`.
- **Test**
  - Test your routes, monitor metrics, and run `glooctl check` until you are happy with the installation.
- **Validate**
  - `gloooctl uninstall -n gloo-system`, or `helm delete` the original installation.

##### Appendix: In-Place "Canary" using XDS-Relay

A simple way to decouple the lifecycle of your control plane from your data plane (installed together by default
with Gloo Edge and Gloo Edge Enterprise) is to use the [xds-relay]({{< versioned_link_path fromRoot="/operations/production_deployment/#xds-relay" >}})
project as your "canary" control plane. This provides extra resiliency for your live xds configuration in the event of
failure during an in-place `helm upgrade`. 