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

**What happens with CRDs when I perform a canary upgrade?**

Each patch version might add custom resource definitions (CRDs), update existing CRDs, or remove outdated CRDs. When you perform a canary upgrade by installing a newer version of Gloo Edge in your data plane cluster, the existing Gloo Edge CRDs are not updated to the newer version automatically, so you must manually apply the new CRDs first. To check the updates to CRDs, view the [upgrade notice for each minor version]({{< versioned_link_path fromRoot="/operations/upgrading/" >}}), or the [changelogs for each patch version]({{< versioned_link_path fromRoot="/reference/changelog/" >}}).

The Gloo Edge CRDs are designed to be backward compatible, so the updated CRDs should not impact the performance of your older installation. However, if after evaluating the newer installation you decide to continue to use the older installation, you can easily remove any added CRDs by referring to the upgrade notices for the CRD names and running `kubectl delete crd <CRD>`. Then, to re-apply older versions of CRDs, you can run `helm pull gloo/gloo --version <older_version> --untar` and `kubectl apply -f gloo/crds`.

## Simple canary upgrades (recommended approach)

1. Apply the new and updated CRDs for the newer version.
   ```sh
   helm pull gloo/gloo --version <version> --untar
   kubectl apply -f gloo/crds
   ```
2. Install the newer version of Gloo Edge in another namespace in your data plane cluster, such as with the following command.
    ```shell
     glooctl install gateway --version <version> -n gloo-system-<version>
     ```
3. Test your routes and monitor the metrics of the newer version.
    ```shell
    glooctl check
    ```
4. Remove the older version of Gloo Edge so that your cluster uses the newer version going forward.
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