---
title: Canary upgrade
weight: 30
description: Use a canary upgrade model to upgrade Gloo Edge, such as in production environments.
---

Use a canary upgrade model to upgrade Gloo Edge, such as in production environments.

In the canary model, you deploy two different version of `gloo` in your data plane: the existing deployment that runs your current version, and a canary deployment that runs the version you want to upgrade to. Then, you verify that the canary deployment handles traffic as you expect before upgrading to the new version. This approach helps you reduce potential downtime for production upgrades.

## Step 1: Prepare to upgrade {#prepare}

Before you begin, follow the [Prepare to upgrade]({{% versioned_link_path fromRoot="/operations/upgrading/faq" %}}) guide to complete these preparatory steps:
* Review important changes made to Gloo Edge in version {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}}, including CRD, Helm, CLI, and feature changes.
* Upgrade your current version to the latest patch.
* Upgrade any dependencies to the required supported versions.
* Consider other steps to prepare for upgrading.
* Review frequently-asked questions about the upgrade process.

## Step 2: Upgrade glooctl {#glooctl}

Follow the steps in [Update glooctl CLI version]({{% versioned_link_path fromRoot="/installation/preparation/#update-glooctl" %}}) to update `glooctl` to the version you want to upgrade to.

## Step 3: Upgrade Gloo Edge {#upgrade}

Now you're ready to upgrade. The steps vary depending on your Gloo Edge installation.
* [Open Source or Enterprise](#canary-upgrade)
* [Federation](#canary-upgrade-fed)

### Upgrade Gloo Edge Open Source or Enterprise {#canary-upgrade}

1. Set the version to upgrade Gloo Edge to in an environment variable, such as the latest patch version for open source (`{{< readfile file="static/content/version_geoss_latest.md" markdown="true">}}`) or enterprise (`{{< readfile file="static/content/version_gee_latest.md" markdown="true">}}`).
   ```sh
   export CANARY_VERSION=<version>
   ```

2. Update and pull the Gloo Edge Helm chart for the canary version.
   {{< tabs >}} 
   {{< tab name="Open Source" >}}
   ```sh
   helm repo add gloo https://storage.googleapis.com/solo-public-helm
   helm repo update
   helm pull gloo/gloo --version $CANARY_VERSION --untar
   ```
   {{< /tab >}}
   {{< tab name="Enterprise">}}
   ```sh
   helm repo add glooe https://storage.googleapis.com/gloo-ee-helm
   helm repo update
   helm pull glooe/gloo-ee --version $CANARY_VERSION --untar
   ```
   {{< /tab >}}
   {{< /tabs >}}

3. Apply the CRDs for the canary version to your cluster. The Gloo Edge CRDs are designed to be backward compatible, so the new CRDs should not impact the performance of your existing installation. However, if after evaluating the canary installation you decide to continue to use the existing installation, you can easily remove any added CRDs by referring to the listed changes to CRD names and running `kubectl delete crd <CRD>`. Then, to re-apply previous versions of CRDs, you can run `helm pull gloo/gloo --version <previous_version> --untar` and `kubectl apply -f gloo/crds`.
   {{< tabs >}} 
   {{< tab name="Open Source" >}}
   ```sh
   kubectl apply -f gloo/crds
   ```
   {{< /tab >}}
   {{< tab name="Enterprise">}}
   ```sh
   kubectl apply -f gloo-ee/charts/gloo/crds
   ```
   {{< /tab >}}
   {{< /tabs >}}

4. Install the canary version of Gloo Edge in your cluster in a new namespace, `gloo-system-$CANARY_VERSION`.
   {{< tabs >}} 
   {{< tab name="Open Source" >}}
   ```sh
   glooctl install gateway \
   --create-namespace \
   -n gloo-system-$CANARY_VERSION \
   --version $CANARY_VERSION
   ```
   {{< /tab >}}
   {{< tab name="Enterprise">}}
   Note that you must set your license key by using the `--license-key $LICENSE_KEY` flag, using the `--set-string license_key=$LICENSE_KEY` flag, or including the `license_key: $LICENSE_KEY` setting in your values file. If you do not have a license key, [request a Gloo Edge Enterprise trial](https://www.solo.io/gloo-trial).
   ```sh
   glooctl install gateway enterprise \
   --create-namespace \
   -n gloo-system-$CANARY_VERSION \
   --version $CANARY_VERSION \
   --license-key $LICENSE_KEY
   ```
   {{< /tab >}}
   {{< /tabs >}}

5. Verify that your existing and canary versions of Gloo Edge are running.
   ```shell
   kubectl get all -n gloo-system
   kubectl get all -n gloo-system-$CANARY_VERSION
   ```

6. Check the [Feature changes]({{% versioned_link_path fromRoot="/operations/upgrading/faq/#features" %}}) to modify any custom resources for any changes or new capabilities in {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}}.<!--If applicable, add steps to walk users though updating crs for any breaking changes-->

7. Test your routes and monitor the metrics of the canary version.
    ```shell
    glooctl check
    ```

8. [Uninstall]({{< versioned_link_path fromRoot="/reference/cli/glooctl_uninstall/" >}}) the previous version of Gloo Edge so that your cluster uses the new version going forward.
   ```shell
   glooctl uninstall -n gloo-system
   ```

### Upgrade Gloo Edge Federation {#canary-upgrade-fed}

In the canary upgrade model for Gloo Edge Federation, you start with an existing version of Gloo Edge Federation in your management cluster that manages one or more remote Gloo Edge instances. To test a new version, you install the canary version of Gloo Edge Federation in a new namespace on the management cluster. Then, you install one or more Gloo Edge instances that run the canary version in remote test clusters, register the test clusters, and check your setup. Finally, you deregister the remote test clusters from the previous version of Gloo Edge Federation, and uninstall the previous Gloo Edge Federation version from the management cluster.

1. Set your Gloo Edge license key in an environment variable. If you do not have a license key, [request a Gloo Edge Enterprise trial](https://www.solo.io/gloo-trial).
   ```sh
   export LICENSE_KEY=<version>
   ```
   
2. Set the version to upgrade Gloo Edge to in an environment variable, such as the latest patch version (`{{< readfile file="static/content/version_gee_latest.md" markdown="true">}}`).
   ```sh
   export CANARY_VERSION=<version>
   ```

3. Update and pull the Gloo Edge Federation Helm chart for the canary version.
   ```shell
   helm repo add gloo-fed https://storage.googleapis.com/gloo-fed-helm
   helm repo update
   helm pull gloo-fed/gloo-fed --version $CANARY_VERSION --untar
   ```

4. Apply the CRDs for the canary version to your management cluster. The Gloo Edge CRDs are designed to be backward compatible, so the new CRDs should not impact the performance of your existing installation. However, if after evaluating the canary installation you decide to continue to use the existing installation, you can easily remove any added CRDs by referring to the listed changes to CRD names and running `kubectl delete crd <CRD>`. Then, to re-apply previous versions of CRDs, you can run `helm pull gloo/gloo --version <previous_version> --untar` and `kubectl apply -f gloo/crds`.
   ```
   kubectl apply -f gloo-fed/crds
   ```

5. Install the canary version of Gloo Edge Federation in a new namespace, `gloo-system-$CANARY_VERSION`, in your management cluster.
   ```
   helm install gloo-fed gloo-fed/gloo-fed \
   --create-namespace \
   -n gloo-system-$CANARY_VERSION \
   --version $CANARY_VERSION \
   --set-string license_key=$LICENSE_KEY
   ```

6. Verify that your existing and canary versions of Gloo Edge Federation are running. 
   ```shell
   kubectl get all -n gloo-system
   kubectl get all -n gloo-system-$CANARY_VERSION
   ```

7. [Install and register Gloo Edge Enterprise]({{< versioned_link_path fromRoot="/guides/gloo_federation/cluster_registration/" >}}) on one of your remote clusters. Make sure that the version of Gloo Edge Enterprise matches the canary version of Gloo Edge Federation that you installed.

8. Check the [Feature changes]({{% versioned_link_path fromRoot="/operations/upgrading/faq/#features" %}}) for any changes or new capabilities in {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}}. In your management cluster, [create or modify any federated custom resources]({{< versioned_link_path fromRoot="/guides/gloo_federation/federated_configuration/" >}}) with these changes to test your canary version of Gloo Edge Federation. Because Gloo Edge Federation scans all federated resources on the management cluster, you might want to reuse the existing federated resources in the `gloo-system` namespace. <!--If applicable, add steps to walk users though updating crs for any breaking changes--> 
   {{% expand "Expand for an example federated virtual service configuration" %}}
   This example configuration creates a federated virtual service with different features across versions 1.12 and 1.13.<!--IF POSSIBLE UPDATE for something that is new in 1.15 instead-->
   * Creates a shared `gloo-fed` namespace to show that both the existing and canary Gloo Edge Federation instances can watch for changes to a resource in the same namespace.
   * Places the federated virtual service resource in both the `remote1` existing version cluster (1.12) and the `remote2` canary version cluster (1.13).
   * Configures settings such as `numRetries: 10` that are shared across versions.
   * Adds the `retryBackOff` that is available in version 1.13 but not in 1.12. As such, the 1.12 federated resource ignores this new configuration.

     ```yaml
     kubectl create ns gloo-fed
     kubectl apply -f - <<EOF
     apiVersion: fed.gateway.solo.io/v1
     kind: FederatedVirtualService
     metadata:
       name: my-fed-vs
       namespace: gloo-fed
     spec:
       placement:
         clusters:
           - remote1
           - remote2
         namespaces:
           - gloo-system
       template:
         metadata:
           name: my-vs
         spec:
           virtualHost:
             domains:
             - '*'
             routes:
             - matchers:
               - prefix: /
               routeAction:
                 single:
                   upstream:
                     name: default-petstore-8080
                     namespace: gloo-system
               options:
                 retries:
                   retryOn: 'connect-failure'
                   numRetries: 10
                   perTryTimeout: '5s'
                   retryBackOff:
                     baseInterval: 1s
                     maxInterval: 3s
     EOF
     ```
   {{% /expand %}}

9. Check that the updated, federated custom resources from the management cluster are propagated to the remote test cluster.
   1. In the management cluster, verify that the federated resources are propagated to the remote clusters by reviewing the federated resource configuration. This example gets the configuration for the federated virtual service from the previous step.
      ```sh
      kubectl get federatedvirtualservice -n gloo-fed -o yaml
      ```
      In this example output, the `PLACED` state in the status section confirms that the resource is federated:
      {{< highlight yaml "hl_lines=10 16" >}} 
...
status:
  namespacedPlacementStatuses:
    gloo-system-$CANARY_VERSION:
      clusters:
        remote2:
          namespaces:
            gloo-system:
              state: PLACED
    gloo-system:
      clusters:
        remote1:
          namespaces:
            gloo-system:
              state: PLACED
      {{< /highlight >}}
   1. In the remote clusters, confirm that the federated resources are created. The following commands are based on the previous step's example.<!--If step 8 was updated for 1.15, update this step as well for the 1.15 feature-->
      * In the existing version's remote cluster, verify that the resource is federated with only the existing version's configuration. In the example, the existing 1.12 version has an updated `numRetries: 10`, but no `retryBackOff` section, which is available only in version 1.13 or later.
        {{< highlight yaml "hl_lines=5" >}}
kubectl get virtualservices -n gloo-system --context remote1 -o yaml
...
options:
  retries:
    numRetries: 10
    perTryTimeout: 5s
    retryOn: connect-failure
        {{< /highlight >}}
      * In the canary version's remote cluster, verify that the resource is federated with only the canary version's configuration. In the example, the canary 1.13 version has an updated `numRetries: 10`, as well as a `retryBackOff` section.
        {{< highlight yaml "hl_lines=6 9-11" >}} 
kubectl get virtualservices -n gloo-system --context remote2 -o yaml
...
options:
  retries:
    numRetries: 10
    perTryTimeout: 5s
    retryOn: connect-failure
    retryBackOff:
      baseInterval: 1s
      maxInterval: 3s
        {{< /highlight >}}
   1. Repeat the previous steps for each federated resource that you want to check.

1.  Test your routes and monitor the metrics of the canary version in your remote test cluster.
    ```shell
    glooctl check
    ```

2.  Shift traffic to the canary version of Gloo Edge Federation by uninstalling the previous version.
    1. [Deregister]({{< versioned_link_path fromRoot="/reference/cli/glooctl_cluster_deregister/" >}}) your other remote clusters that still use the previous version of Gloo Edge Federation.
    2. [Uninstall]({{< versioned_link_path fromRoot="/reference/cli/glooctl_uninstall/" >}}) the previous version of Gloo Edge Federation so that your management cluster uses only the new version.
       ```shell
       glooctl uninstall -n gloo-system
       ```
    3. Optionally delete the previous version namespace from each remote cluster. The deregister command does not clean up the namespace and custom resources in the previous version.
       ```shell
       kubectl delete ns gloo-system
       ```

## Roll back a canary upgrade {#rollback}

As you test your environment with the canary version of Gloo Edge, you might need to roll back to the previous version. You can follow a canary model for the rollback.

1. Set the previous version that you want to roll back to as an environment variable.
   ```shell
   export ROLLBACK_VERSION=<version>
   ```
2. If you already removed the previous version of Gloo Edge from your cluster, re-install Gloo Edge at the rollback version.
   1. Update and pull the Gloo Edge Helm chart for the rollback version. The Helm charts vary depending on the Gloo Edge installation option.
      {{< tabs >}} 
{{< tab name="Open Source" >}}
```sh
helm repo update
helm pull gloo/gloo --version $ROLLBACK_VERSION --untar
```
{{< /tab >}}
{{< tab name="Enterprise">}}
```sh
helm repo update
helm pull glooe/gloo-ee --version $ROLLBACK_VERSION --untar
```
{{< /tab >}}
{{< tab name="Federation">}}
```sh
helm repo update
helm pull gloo-fed/gloo-fed --version $ROLLBACK_VERSION --untar
```
{{< /tab >}} 
      {{< /tabs >}}
   2. Install the rollback version of Gloo Edge in a new namespace in your cluster.
      {{< tabs >}}
{{< tab name="Open Source" >}}
```sh
glooctl install gateway \
--create-namespace \
--version $ROLLBACK_VERSION \
-n gloo-system-$ROLLBACK_VERSION
```
{{< /tab >}}
{{< tab name="Enterprise">}}
```sh
glooctl install gateway enterprise \
--create-namespace \
--version $ROLLBACK_VERSION \
-n gloo-system-$ROLLBACK_VERSION \
--license-key $LICENSE_KEY
```
{{< /tab >}}
{{< tab name="Federation">}}
```sh
helm install gloo-fed gloo-fed/gloo-fed \
--create-namespace \
-n gloo-system-$ROLLBACK_VERSION \
--version $ROLLBACK_VERSION \
--set-string license_key=$LICENSE_KEY
```
{{< /tab >}} 
      {{< /tabs >}}

1. Revert any changes to custom resources that you made during the upgrade. For differences between versions, check the [version change list]({{< versioned_link_path fromRoot="/operations/upgrading/faq/#review-changes" >}}) and the [changelogs]({{< versioned_link_path fromRoot="/reference/changelog/" >}}).

2. **Gloo Edge Federation**: Shift traffic from the canary version to the rollback version of Gloo Edge Federation.
   1. [Deregister]({{< versioned_link_path fromRoot="/reference/cli/glooctl_cluster_deregister/" >}}) your test remote clusters that use the canary version of Gloo Edge Federation.
   2. [Install and register Gloo Edge Enterprise]({{< versioned_link_path fromRoot="/guides/gloo_federation/cluster_registration/" >}}) using the rollback version on each remote cluster.
   3. Optionally delete the canary version namespace. The deregister command does not clean up the namespace and custom resources in the previous version.
      ```shell
      kubectl delete ns gloo-system-$CANARY_VERSION
      ```

3. [Uninstall]({{< versioned_link_path fromRoot="/reference/cli/glooctl_uninstall/" >}}) the canary version of Gloo Edge from your cluster.
    ```shell
    glooctl uninstall -n gloo-system-$CANARY_VERSION
    ```

4. Apply the Gloo CRDs from the rollback version Helm chart to your cluster. In Gloo Edge Federation, this cluster is the local management cluster.
   {{< tabs >}} 
{{< tab name="Open Source" >}}
kubectl apply -f gloo/crds
{{< /tab >}}
{{< tab name="Enterprise">}}
kubectl apply -f gloo-ee/charts/gloo/crds
{{< /tab >}}
{{< tab name="Federation">}}
kubectl apply -f gloo-fed/crds
{{< /tab >}} 
   {{< /tabs >}}

1. Test your routes and monitor the metrics of the rollback version.
   ```shell
   glooctl check
   ```