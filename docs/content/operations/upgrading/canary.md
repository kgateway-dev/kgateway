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

##### Install the second Gloo control plane

