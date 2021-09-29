---
title: Canary Upgrade (1.9.0+)
weight: 30
description: Upgrading Gloo Edge with a canary workflow
---

In version 1.9.0 or later, you can upgrade your Gloo Edge or Gloo Edge Enterprise deployments with a canary model. In
the canary model, you have two different `gloo` deployments in your data plane and can check that the deployment at the
latest version handles traffic as you expect before upgrading to run at the latest version.

## Prereqs

Gloo Edge 1.9.0 or later installed.

{{% notice note %}}

**Why can't I perform canary upgrades for versions older than 1.9?** Prior to 1.9.0, status reporting on Gloo CRs was
not per namespace, so you could not monitor the state of separate Gloo resources at different versions in different
namespaces as part of a canary upgrade. Also, the Helm configuration could not override the xDS service address on
xDS service port, such as the following:

```yaml
gatewayProxies:
  gatewayProxy: # do the following for each gateway proxy
    xdsServiceAddress: xds-relay.default.svc.cluster.local
    xdsServicePort: 9991
```

Prior to 1.8.0, a [bug](https://github.com/solo-io/gloo/issues/5030) prevented canary deployments
from working because the old control plane crash went into a crash loop when new fields were added.

{{% /notice %}}

## Simple canary upgrades (recommended approach)

1. Install the newer version of Gloo Edge in another namespace in your data plane cluster, such as with the following command.
    ```shell
     glooctl install gateway --version 1.9.1 -n gloo-system-1-9-1
     ```
2. Test your routes and monitor the metrics of the newer version.
    ```shell
    glooctl check
    ```
3. Remove the older version of Gloo Edge so that your cluster uses the newer version going forward.
   With `glooctl`:
    ```shell
    gloooctl uninstall -n gloo-system
    ```
   With Helm:
    ```shell
    helm delete
    ```

## Appendix: In-place canary upgrades by using xDS relay

By default, your Gloo Edge or Gloo Edge Enterprise control plane and data plane are installed together. However, you can
decouple the control and data plane' lifecycle by using the [`xds-relay`]({{< versioned_link_path fromRoot="/operations/production_deployment/#xds-relay" >}})
project as the "control plane" for the newer version deployment in a canary upgrade. This setup provides extra
resiliency for your live xDS configuration in the event of failure during an in-place `helm upgrade`. 