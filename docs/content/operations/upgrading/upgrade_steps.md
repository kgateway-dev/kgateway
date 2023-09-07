---
title: Quick upgrade
weight: 20
description: Quickly upgrade Gloo Edge in testing or sandbox environments.
---

Quickly upgrade your Gloo Edge Enterprise or Gloo Edge Open Source installation to the latest version of {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}} in testing or sandbox environments.

## Step 1: Prepare to upgrade {#prepare}

Before you begin, follow the [Prepare to upgrade]({{% versioned_link_path fromRoot="/operations/upgrading/faq" %}}) guide to complete these preparatory steps:
* Review important changes made to Gloo Edge in version {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}}, including CRD, Helm, CLI, and feature changes.
* Upgrade your current version to the latest patch.
* Upgrade any dependencies to the required supported versions.
* Consider other steps to prepare for upgrading.
* Review frequently-asked questions about the upgrade process.

## Step 2: Upgrade glooctl {#glooctl}

Follow the steps in [Update glooctl CLI version]({{% versioned_link_path fromRoot="/installation/preparation/#update-glooctl" %}}) to update `glooctl` to the version you want to upgrade to.

## Step 3: Apply minor version-specific changes {#crds}

Each minor version might add custom resource definitions (CRDs) or otherwise have changes that Helm upgrades cannot handle seamlessly. For these changes, you must make any necessary adjustments before you upgrade.

1. Update the Gloo Edge Helm repositories.
   ```sh
   helm repo update
   ```

2. Set the version to upgrade Gloo Edge to in an environment variable, such as the latest patch version for open source (`{{< readfile file="static/content/version_geoss_latest.md" markdown="true">}}`) or enterprise (`{{< readfile file="static/content/version_gee_latest.md" markdown="true">}}`).
   ```sh
   export NEW_VERSION=<version>
   ```
   {{% notice note %}}
   When you upgrade to 1.15.x, choose a patch version that is later than 1.15.0, such as `{{< readfile file="static/content/version_geoss_latest.md" markdown="true">}}`. 1.15.0 contains a [bug](https://github.com/solo-io/gloo/issues/8627) that is fixed in 1.15.1 and later patches.
   {{% /notice %}}

3. Check the [CRD changes]({{% versioned_link_path fromRoot="/operations/upgrading/faq/#crd" %}}) to see which CRDs are new, deprecated, or removed in version {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}}.
   1. Delete any removed CRDs. <!--If applicable, add commands to kubectl delete the removed CRDs-->
   2. Apply the new and updated CRDs.
      {{< tabs >}}
{{% tab name="Open Source" %}}
```sh
helm pull gloo/gloo --version $NEW_VERSION --untar
kubectl apply -f gloo/crds
```
{{% /tab %}}
{{% tab name="Enterprise" %}}
```sh
helm pull glooe/gloo-ee --version $NEW_VERSION --untar
kubectl apply -f gloo-ee/charts/gloo/crds
# If Gloo Federation is enabled
kubectl apply -f gloo-ee/charts/gloo-fed/crds
```
{{% /tab %}}
      {{< /tabs >}}
   1. Verify that the deployed CRDs use the version that you want to upgrade to.
      ```
      glooctl check-crds
      ```

1. Check the [Feature changes]({{% versioned_link_path fromRoot="/operations/upgrading/faq/#features" %}}) to see whether there are breaking changes you must address in your resources before you upgrade to {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}}. <!--If applicable, add steps to walk users though updating crs for any breaking changes-->

2. Check the [Helm changes]({{% versioned_link_path fromRoot="/operations/upgrading/faq/#helm" %}}) to see whether there are new, deprecated, or removed Helm settings you might address before you upgrade to {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}}.
   1. Get the Helm values file for your current installation.
      {{< tabs >}}
{{% tab name="Open Source" %}}
```sh
helm get values -n gloo-system gloo gloo/gloo > values.yaml
open values.yaml
```
{{% /tab %}}
{{% tab name="Enterprise" %}}
```sh
helm get values -n gloo-system gloo glooe/gloo-ee > values.yaml
open values.yaml
```
{{% /tab %}}
      {{< /tabs >}}
   1. Edit the Helm values file or prepare the `--set` flags to make any changes that you want. If you do not want to use certain settings, comment them out.

## Step 4: Upgrade Gloo Edge {#upgrade}

Upgrade your Gloo Edge installation. The following example upgrade commands assume that Gloo Edge is installed with Helm, the Helm release exists in the `gloo-system` namespace of a Kubernetes cluster that uses the Kubernetes load balancer, and that the Kubernetes context is set to the cluster.

1. Upgrade the Helm release. Include your installation values in a Helm values file (such as `-f values.yaml`) or in `--set` flags.
   {{< tabs >}}
   {{% tab name="Open Source" %}}
   ```shell script
   helm upgrade -n gloo-system gloo gloo/gloo \
     -f values.yaml \
     --version=$NEW_VERSION
   ```
   {{% /tab %}}
   {{% tab name="Enterprise" %}}
   Note that you must set your license key by using the `--set-string license_key=$LICENSE_KEY` flag or including the `license_key: $LICENSE_KEY` setting in your values file. If you do not have a license key, [request a Gloo Edge Enterprise trial](https://www.solo.io/gloo-trial).
   ```shell script
   helm upgrade -n gloo-system gloo glooe/gloo-ee \
     -f values.yaml \
     --version=$NEW_VERSION \
     --set license_key=$LICENSE_KEY
   ```
   {{% /tab %}}
   {{< /tabs >}}

2. Verify that Gloo Edge runs the upgraded version.
   ```shell script
   kubectl -n gloo-system get pod -l gloo=gloo -ojsonpath='{.items[0].spec.containers[0].image}'
   ```

   Example output:
   ```
   quay.io/solo-io/gloo:{{< readfile file="static/content/version_geoss_latest.md" markdown="true">}}
   ```

3. Verify that all server components run the upgraded version.
   ```shell script
   glooctl version
   ```

4. Check that your Gloo Edge components are **OK**. If a problem is reported by `glooctl check`, Gloo Edge might not work properly or Envoy might not get the updated configuration.
   ```bash
   glooctl check
   ```
   Example output:
   ```bash
   Checking deployments... OK
   Checking pods... OK
   Checking upstreams... OK
   Checking upstream groups... OK
   Checking secrets... OK
   Checking virtual services... OK
   Checking gateways... OK
   Checking proxies... OK
   No problems detected.
   ```

5. Now that your upgrade is complete, you can enable any other [new features]({{% versioned_link_path fromRoot="/operations/upgrading/faq/#features" %}}) in version {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}} that you want to use.