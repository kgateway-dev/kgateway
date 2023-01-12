---
title: Canary Upgrade
weight: 30
description: Upgrading Gloo Edge in a canary model
---

You can upgrade your Gloo Edge deployments by following a canary model. In the canary model, you make two different `gloo` deployments in your data plane, one that runs your current version and one for the target version to upgrade to. Then, you check that the deployment at the target version handles traffic as you expect before upgrading to run the target version. This approach helps you reduce potential downtime for production upgrades.

## Before you begin

1. Install Gloo Edge Open Source or Enterprise **version 1.9.0 or later**, or Federation **version 1.13.0 or later**.
2. [Upgrade your installation]({{< versioned_link_path fromRoot="/operations/upgrading/upgrade_steps/" >}}) to the latest patch version for your current minor version. For example, you might upgrade your {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}} installation to the latest {{< readfile file="static/content/version_geoss_latest.md" markdown="true">}} patch.
3. If you have Gloo Edge Enterprise or Federation, set your license key as an environment variable. To request a license, [contact Sales](https://www.solo.io/company/contact/).
   ```
   export GLOO_LICENSE=<license>
   ```
4. Set the target version that you want to upgrade to as an environment variable. To find available versions, check the [changelog]({{< versioned_link_path fromRoot="/reference/changelog/" >}}). The following commands include the latest versions for each of the following Gloo Edge installation options.
   {{< tabs >}} 
{{< tab name="Open Source" codelang="shell" >}}
export TARGET_VERSION={{< readfile file="static/content/version_geoss_latest.md" markdown="true">}}
{{< /tab >}}
{{< tab name="Enterprise" codelang="shell" >}}
export TARGET_VERSION={{< readfile file="static/content/version_gee_latest.md" markdown="true">}}
{{< /tab >}} 
{{< tab name="Federation" codelang="shell" >}}
export TARGET_VERSION={{< readfile file="static/content/version_gee_latest.md" markdown="true">}}
{{< /tab >}} 
   {{< /tabs >}}
5. Upgrade your `glooctl` CLI to the version that you want to upgrade to.
   ```
   glooctl upgrade --release v${TARGET_VERSION}
   ```
6. Check the [upgrade notice for the minor version]({{< versioned_link_path fromRoot="/operations/upgrading/" >}}) and the [changelogs for the patch version]({{< versioned_link_path fromRoot="/reference/changelog/" >}}) that you want to upgrade to. In particular, review the following changes:
   * **CRD changes**: Each patch version might add custom resource definitions (CRDs), update existing CRDs, or remove outdated CRDs. When you perform a canary upgrade, the existing Gloo Edge CRDs are not updated to the newer version automatically. You must manually apply the new CRDs first. The Gloo Edge CRDs are designed to be backward compatible, so the new CRDs should not impact the performance of your older installation. However, if after evaluating the newer installation you decide to continue to use the older installation, you can easily remove any added CRDs by referring to the upgrade notices for the CRD names and running `kubectl delete crd <CRD>`. Then, to re-apply older versions of CRDs, you can run `helm pull gloo/gloo --version <older_version> --untar` and `kubectl apply -f gloo/crds`.
   * **Breaking changes**: Occasionally, breaking changes are introduced between patch versions. For example, Gloo custom resources might get renamed or have conflicting fields. Modify your resources to use the new version that you upgrade to.

Now you're ready to upgrade your Gloo Edge installation. The steps vary depending on if you use [Open Source or Enterprise](#canary-upgrade), or [Federation](#canary-upgrade-fed).

## Upgrade Gloo Edge Open Source or Enterprise in a canary model {#canary-upgrade}

1. Update and pull the Gloo Edge Helm chart for the target version.
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
   {{< /tabs >}}
1. Apply the new Gloo CRDs from the target version Helm chart to your cluster.
   {{< tabs >}} 
{{< tab name="Open Source" codelang="shell" >}}
kubectl apply -f gloo/crds
{{< /tab >}}
{{< tab name="Enterprise" codelang="shell">}}
kubectl apply -f gloo-ee/charts/gloo/crds
{{< /tab >}}
   {{< /tabs >}}
3. Install the target version of Gloo Edge in the new namespace in your cluster.
   {{< tabs >}} 
{{< tab name="Open Source" codelang="shell" >}}
glooctl install gateway --version $TARGET_VERSION -n gloo-system-$TARGET_VERSION
{{< /tab >}}
{{< tab name="Enterprise" codelang="shell">}}
glooctl install gateway enterprise --version $TARGET_VERSION -n gloo-system-$TARGET_VERSION --license-key $GLOO_LICENSE
{{< /tab >}}
   {{< /tabs >}}
4. Verify that your current and target versions of Gloo Edge are running.
   ```shell
   kubectl get all -n gloo-system
   kubectl get all -n gloo-system-$TARGET_VERSION
   ```
5. Modify any custom resources for any changes or new capabilities that you noticed in the [upgrade notice]({{< versioned_link_path fromRoot="/operations/upgrading/" >}}) and the [changelogs]({{< versioned_link_path fromRoot="/reference/changelog/" >}}) for the target version.
6. Test your routes and monitor the metrics of the newer version.
    ```shell
    glooctl check
    ```
7. [Uninstall]({{< versioned_link_path fromRoot="/reference/cli/glooctl_uninstall/" >}}) the older version of Gloo Edge so that your cluster uses the newer version going forward.
   ```shell
   gloooctl uninstall -n gloo-system
   ```

## Upgrade Gloo Edge Federation in a canary model {#canary-upgrade-fed}

You can upgrade Gloo Edge Federation in a canary model in version 1.13 or later. In [Gloo Edge Federation]({{< versioned_link_path fromRoot="/guides/gloo_federation/" >}}), you have a management cluster that runs Gloo Edge Federation (and optionally, other Enterprise components). Then you register remote clusters with Gloo Edge Enterprise.

In the canary upgrade model, you start with an older version of Gloo Edge Federation in your management cluster, managing one or more remote Gloo Edge instances. To test a newer target version, you install the target version of Gloo Edge Federation in a new namespace on the management cluster. Then, you install one or more Gloo Edge instances at the target version in remote test clusters, register the new clusters, and check your setup. Finally, you deregister the remote clusters from the older version of Gloo Edge Federation, register the remote clusters and uninstall the older Gloo Edge Federation version.

1. Update and pull the Gloo Edge Helm chart for the target version.
   ```shell
   helm repo add gloo-fed https://storage.googleapis.com/gloo-fed-helm
   helm repo update
   helm pull gloo-fed/gloo-fed --version $TARGET_VERSION --untar
   ```
2. Apply the new Gloo CRDs from the target version Helm chart to your cluster. In Gloo Edge Federation, this cluster is the local management cluster.
   ```
   kubectl apply -f gloo-fed/charts/gloo/crds
   ```
3. Install the target version of Gloo Edge Federation in the new namespace in your management cluster.
   ```
   helm install -n gloo-system-$TARGET_VERSION gloo-fed gloo-fed/gloo-fed --create-namespace --set-string license_key=$GLOO_LICENSE --version $TARGET_VERSION
   ```
4. Verify that your current and target versions of Gloo Edge Federation are running. 
   ```shell
   kubectl get all -n gloo-system-$OLD_VERSION
   kubectl get all -n gloo-system-$TARGET_VERSION
   ```
5. [Install and register Gloo Edge Enterprise]({{< versioned_link_path fromRoot="/guides/gloo_federation/cluster_registration/" >}}) on one of your remote clusters. Make sure that the version of Gloo Edge Enterprise matches the version of Gloo Edge Federation that you installed earlier.
6. In your management cluster, [create any federated custom resources]({{< versioned_link_path fromRoot="/guides/gloo_federation/federated_configuration/" >}}) for the target version in your `gloo-system-$TARGET_VERSION` namespace. You might use the existing federated resources in the `gloo-system-$OLD_VERSION` namespace as a starting point. Include with any changes or new capabilities that you noticed in the [upgrade notice]({{< versioned_link_path fromRoot="/operations/upgrading/" >}}) and the [changelogs]({{< versioned_link_path fromRoot="/reference/changelog/" >}}) for the target version.
7. Check that the updated, federated custom resources from the management cluster are propagated to the remote test cluster.
   1. To find the clusters and names of the propagated resources, review the federated resource configuration. In the following example, a `fed-upstream` upstream is federated in `remote1` and `remote2` clusters in the `gloo-system-new` namespace. The `PLACED` state in the status section confirms that the resource is federated.
      ```sh
      kubectl get federatedupstream -n gloo-system-$TARGET_VERSION -o yaml
      ```
      Example truncated output:
      ```yaml
      ...
      spec:
        placement:
          clusters:
            - remote1
            - remote2
          namespaces:
            - gloo-system-new
        template:
          metadata:
            name: fed-upstream
      ...
      status:
        placementStatus:
          clusters:
            local:
              namespaces:
                gloo-system:
                  state: PLACED
      ```
   2. In the remote cluster, confirm that the federated resources are created. The following command is based on the previous step's example.
      ```sh
      kubectl get upstreams -n gloo-system-new --context remote1
      kubectl get upstreams -n gloo-system-new --context remote2
      ```
   3. Repeat the previous steps for each federated resource that you want to check.
8. Test your routes and monitor the metrics of the target version in your remote test cluster.
    ```shell
    glooctl check
    ```
9.  Shift traffic to the target version of Gloo Edge Federation.
   1. [Deregister]({{< versioned_link_path fromRoot="/reference/cli/glooctl_cluster_deregister/" >}}) your other remote clusters that still use the old version of Gloo Edge Federation.
   2. [Install and register Gloo Edge Enterprise]({{< versioned_link_path fromRoot="/guides/gloo_federation/cluster_registration/" >}}) on each of your remote clusters. Make sure that the version of Gloo Edge Enterprise matches the version of Gloo Edge Federation that you installed earlier.
   3. Optionally delete the old version namespace from each remote cluster. The deregister command does not clean up the namespace and custom resources in the old version.
      ```shell
      kubectl delete ns gloo-system-$OLD_VERSION
      ```
10. [Uninstall]({{< versioned_link_path fromRoot="/reference/cli/glooctl_uninstall/" >}}) the older version of Gloo Edge Federation so that your management cluster uses the newer version going forward.
   ```shell
   gloooctl uninstall -n gloo-system-$OLD_VERSION
   ```

## Roll back a canary upgrade {#rollback}

As you test your environment with the new version of Gloo Edge, you might need to roll back to the previous version. You can follow a canary model for the rollback.

1. Set the previous version that you want to roll back to as an environment variable.
   ```shell
   export ROLLBACK_VERSION=<version>
   ```
2. If you already removed the previous version of Gloo from your cluster, re-install Gloo Edge.
   1. Update and pull the Gloo Edge Helm chart for the target version. The Helm charts vary depending on the Gloo Edge installation option.
      {{< tabs >}} 
{{< tab name="Open Source" codelang="shell" >}}
helm repo update
helm pull gloo/gloo --version $ROLLBACK_VERSION --untar
{{< /tab >}}
{{< tab name="Enterprise" codelang="shell">}}
helm repo update
helm pull glooe/gloo-ee --version $ROLLBACK_VERSION --untar
{{< /tab >}}
{{< tab name="Federation" codelang="shell">}}
helm repo update
helm pull gloo-fed/gloo-fed --version $ROLLBACK_VERSION --untar
{{< /tab >}} 
      {{< /tabs >}}
   2. Install the rollback version of Gloo Edge in the new namespace in your cluster.
      {{< tabs >}} 
{{< tab name="Open Source" codelang="shell" >}}
glooctl install gateway --version $ROLLBACK_VERSION-n gloo-system-$ROLLBACK_VERSION
{{< /tab >}}
{{< tab name="Enterprise" codelang="shell">}}
glooctl install gateway enterprise --version $ROLLBACK_VERSION -n gloo-system-$ROLLBACK_VERSION --license-key $GLOO_LICENSE
{{< /tab >}}
{{< tab name="Federation" codelang="shell">}}
helm install -n gloo-system-$ROLLBACK_VERSION gloo-fed gloo-fed/gloo-fed --create-namespace --set-string license_key=$GLOO_LICENSE --version $ROLLBACK_VERSION
{{< /tab >}} 
      {{< /tabs >}}
1.  Revert any changes to custom resources that you previously modified during the upgrade to the newer target version. For differences between versions, check the [upgrade notice]({{< versioned_link_path fromRoot="/operations/upgrading/" >}}) and the [changelogs]({{< versioned_link_path fromRoot="/reference/changelog/" >}}).
2. **Gloo Edge Federation**: Shift traffic from the newer target version to the rollback version of Gloo Edge Federation.
   1. [Deregister]({{< versioned_link_path fromRoot="/reference/cli/glooctl_cluster_deregister/" >}}) your remote clusters that still use the newer target version of Gloo Edge Federation.
   2. [Install and register Gloo Edge Enterprise]({{< versioned_link_path fromRoot="/guides/gloo_federation/cluster_registration/" >}}) for the rollback version's Gloo Edge Federation on each remote cluster.
   3. Optionally delete the newer target version namespace. The deregister command does not clean up the namespace and custom resources in the old version.
      ```shell
      kubectl delete ns gloo-system-$TARGET_VERSION
      ```
3. [Uninstall]({{< versioned_link_path fromRoot="/reference/cli/glooctl_uninstall/" >}}) the newer target version of Gloo Edge from your cluster.
    ```shell
    gloooctl uninstall -n gloo-system-$TARGET_VERSION
    ```
4.  Apply the Gloo CRDs from the rollback version Helm chart to your cluster. In Gloo Edge Federation, this cluster is the local management cluster.
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
7. Test your routes and monitor the metrics of the rollback version.
    ```shell
    glooctl check
    ```

## Appendix: In-place canary upgrades by using xDS relay {#canary-xds-relay}

By default, your Gloo Edge or Gloo Edge Enterprise control plane and data plane are installed together. However, you can decouple the control plane and data plane lifecycles by using the [`xds-relay`]({{< versioned_link_path fromRoot="/operations/advanced/xds_relay/" >}})
project as the "control plane" for the newer version deployment in a canary upgrade. This setup provides extra resiliency for your live xDS configuration in the event of failure during an in-place `helm upgrade`. 