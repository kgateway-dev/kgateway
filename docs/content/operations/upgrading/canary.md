---
title: Canary Upgrade
weight: 30
description: Upgrading Gloo Edge in a canary model
---

You can upgrade your Gloo Edge Open Source, Gloo Edge Enterprise, and Gloo Edge Federation deployments by following a canary model. In the canary model, you make two different `gloo` deployments in your data plane, one that runs your current version and one for the target version to upgrade to. Then, you check that the deployment at the target version handles traffic as you expect before upgrading to run the target version. This approach helps you reduce potential downtime for production upgrades.

## Before you begin

1. Install Gloo Edge Open Source, Enterprise, or Federation **version 1.9.0 or later**. 
2. If you have Gloo Edge Enterprise or Federation, set your license key as an environment variable. To request a license, [contact Sales](https://www.solo.io/company/contact/).
   ```
   export GLOO_LICENSE=<license>
   ```
3. Set the target version that you want to upgrade to as an environment variable. To find available versions, check the [changelog]({{< versioned_link_path fromRoot="/reference/changelog/" >}}). The following commands include the latest versions for each of the following Gloo Edge installation options.
   {{< tabs >}} 
{{< tab name="Open Source" codelang="shell" >}}
export TARGET_VERSION={{< readfile file="static/content/version_geoss_latest" markdown="true">}}
{{< /tab >}}
{{< tab name="Enterprise" codelang="shell" >}}
export TARGET_VERSION={{< readfile file="static/content/version_gee_latest" markdown="true">}}
{{< /tab >}} 
{{< tab name="Federation" codelang="shell" >}}
export TARGET_VERSION={{< readfile file="static/content/version_gee_latest" markdown="true">}}
{{< /tab >}} 
   {{< /tabs >}}
4. Upgrade your `glooctl` CLI to the version that you want to upgrade to.
   ```
   glooctl upgrade --release v${TARGET_VERSION}
   ```
5. Check the [upgrade notice for the minor version]({{< versioned_link_path fromRoot="/operations/upgrading/" >}}) and the [changelogs for the patch version]({{< versioned_link_path fromRoot="/reference/changelog/" >}}) that you want to upgrade to. In particular, review the following changes:
   * **CRD changes**: Each patch version might add custom resource definitions (CRDs), update existing CRDs, or remove outdated CRDs. When you perform a canary upgrade by installing a newer version of Gloo Edge in your data plane cluster, the existing Gloo Edge CRDs are not updated to the newer version automatically, so you must manually apply the new CRDs first. The Gloo Edge CRDs are designed to be backward compatible, so the updated CRDs should not impact the performance of your older installation. However, if after evaluating the newer installation you decide to continue to use the older installation, you can easily remove any added CRDs by referring to the upgrade notices for the CRD names and running `kubectl delete crd <CRD>`. Then, to re-apply older versions of CRDs, you can run `helm pull gloo/gloo --version <older_version> --untar` and `kubectl apply -f gloo/crds`.
   * **Breaking changes**: Occasionally, breaking changes are introduced between patch versions. For example, renamed or reconfigured components or conflicting fields might be added to Gloo custom resources. You might have to modify your resources to use the new version that you upgrade to.

## Upgrade Gloo Edge in a canary model {#canary-upgrade}

1. Update and pull the Gloo Edge Helm chart for the target version. The Helm charts vary depending on the Gloo Edge installation option.
   {{< tabs >}} 
{{< tab name="Open Source" codelang="shell" >}}
helm repo add gloo https://storage.googleapis.com/solo-public-helm
helm repo update
helm pull gloo/gloo --version $TARGET_VERSION --untar
{{< /tab >}}
{{< tab name="Enterprise" codelang="shell">}}
helm repo add glooe https://storage.googleapis.com/gloo-ee-helm
helm repo update
helm pull glooe/gloo-ee --version $TARGET_VERSION --untar
{{< /tab >}}
{{< tab name="Federation" codelang="shell">}}
helm repo add gloo-fed https://storage.googleapis.com/gloo-fed-helm
helm repo update
helm pull gloo-fed/gloo-fed --version $TARGET_VERSION --untar
{{< /tab >}} 
   {{< /tabs >}}
2. Apply the new Gloo CRDs from the target version Helm chart to your cluster. In Gloo Edge Federation, this cluster is the local management cluster.
   {{< tabs >}} 
{{< tab name="Open Source" codelang="shell" >}}
kubectl apply -f gloo/crds
{{< /tab >}}
{{< tab name="Enterprise" codelang="shell">}}
kubectl apply -f gloo-ee/charts/gloo/crds
{{< /tab >}}
{{< tab name="Federation" codelang="shell">}}
kubectl apply -f gloo-fed/charts/gloo/crds
{{< /tab >}} 
   {{< /tabs >}}
3. Create a namespace for the target version of Gloo Edge in your cluster.
   ```shell
   kubectl create ns gloo-system-$TARGET_VERSION
   ```
4. Install the target version of Gloo Edge in the new namespace in your cluster.
   {{< tabs >}} 
{{< tab name="Open Source" codelang="shell" >}}
glooctl install gateway --version $TARGET_VERSION -n gloo-system-$TARGET_VERSION
{{< /tab >}}
{{< tab name="Enterprise" codelang="shell">}}
glooctl install gateway enterprise --version $TARGET_VERSION -n gloo-system-$TARGET_VERSION --license-key $GLOO_LICENSE
{{< /tab >}}
{{< tab name="Federation" codelang="shell">}}
glooctl install gateway enterprise --version $TARGET_VERSION -n gloo-system-$TARGET_VERSION --license-key $GLOO_LICENSE
{{< /tab >}} 
   {{< /tabs >}}
5. Verify that your current and target versions of Gloo Edge are running.
   ```shell
   kubectl get all -n gloo-system
   kubectl get all -n gloo-system-$TARGET_VERSION
   ```
6. **Gloo Edge Federation installation**: [Install and register Gloo Edge]({{< versioned_link_path fromRoot="/guides/gloo_federation/cluster_registration/" >}}) on each of your remote clusters.
7. **HOW to know which? Maybe optional depending on changelog?** Modify federated CRs.
8. Test your routes and monitor the metrics of the newer version.
    ```shell
    glooctl check
    ```
9. **How? Is this a resource you modify** Shift traffic to the target version.
10. Remove the older version of Gloo Edge so that your cluster uses the newer version going forward.
   With `glooctl`:
    ```shell
    gloooctl uninstall -n gloo-system
    ```
   **REMOVE THIS??** With Helm:
    ```shell
    helm delete
    ```

## Appendix: In-place canary upgrades by using xDS relay {#canary-xds-relay}

By default, your Gloo Edge or Gloo Edge Enterprise control plane and data plane are installed together. However, you can
decouple the control plane and data plane lifecycles by using the [`xds-relay`]({{< versioned_link_path fromRoot="/operations/advanced/xds_relay/" >}})
project as the "control plane" for the newer version deployment in a canary upgrade. This setup provides extra
resiliency for your live xDS configuration in the event of failure during an in-place `helm upgrade`. 