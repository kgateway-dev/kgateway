---
title: Upgrade Steps
weight: 10
description: Steps for upgrading Gloo Edge components
---

Upgrade your Gloo Edge Enterprise or Gloo Edge Open Source installations, such as from one minor version to the latest version.

{{% notice warning %}}
Use this guide to upgrade Gloo Edge development or staging environments only. The basic upgrade process is not suitable for environments in which downtime is unacceptable. Additionally, you might need to take steps to account for other factors such as Gloo Edge version changes, probe configurations, and external infrastructure like the load balancer that Gloo Edge uses. For more information, see the [Canary Upgrade]({{% versioned_link_path fromRoot="/operations/upgrading/canary/" %}}) guide.
{{% /notice %}}

## Step 1: Prepare to upgrade

Upgrade your current version to the latest patch, upgrade any dependencies to the required supported versions, and consider other steps to prepare for upgrading.

### Upgrade current version

1. Before you upgrade your minor version, first upgrade your current version to the latest patch. For example, if you currently run Gloo Edge Enterprise version `{{< readfile file="static/content/version_gee_n-1_oldpatch.md" markdown="true">}}`, first upgrade your installation to version `{{< readfile file="static/content/version_gee_n-1.md" markdown="true">}}`. This ensures that your current environment is up-to-date with any bug fixes or security patches before you begin the minor version upgrade process.
   1. Find the latest patch of your minor version by checking the [Open Source changelog]({{% versioned_link_path fromRoot="/reference/changelog/open_source/" %}}) or [Enterprise changelog]({{% versioned_link_path fromRoot="/reference/changelog/enterprise/" %}}).
   2. Go to the documentation set for your current minor version. For example, if you currently run Gloo Edge Enterprise version `{{< readfile file="static/content/version_gee_n-1_oldpatch.md" markdown="true">}}`, use the drop-down menu in the header of this page to select **v{{< readfile file="static/content/version_geoss_n-1_minor.md" markdown="true">}}.x**.
   3. Follow the upgrade guide, using the latest patch for your minor version.
2. If you plan to upgrade to a version that is more than one minor version greater than your current version, such as to version {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}} from 1.13 or older, you must upgrade incrementally. For example, you must first upgrade from 1.13 to 1.14, and then follow this guide to upgrade from 1.14 to {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}}.

### Upgrade dependencies

Check that your underlying infrastructure platform, such as Kubernetes, and other dependencies run a version that is supported for `{{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}}`.
1. Review the [supported versions]({{% versioned_link_path fromRoot="/reference/support/#supported-versions" %}}) for dependencies such as Kubernetes, Helm, and more.
2. Compare the supported versions against the versions you currently use.
3. If necessary, upgrade your dependencies, such as consulting your cluster infrastructure provider to upgrade the version of Kubernetes that your cluster runs.

### Consider settings to avoid downtime

You might deploy Gloo Edge in Kubernetes environments that use the Kubernetes load balancer, or in non-Kubernetes environments. Depending on your setup, you can take additional steps to avoid downtime during the upgrade process.

* **Kubernetes**: Enable [Envoy readiness and liveness probes]({{< versioned_link_path fromRoot="/operations/production_deployment/#enable-health-checks" >}}) during the upgrade. When these probes are set, Kubernetes sends requests only to the healthy Envoy proxy during the upgrade process, which helps to prevent potential downtime. The probes are not enabled in default installations because they can lead to timeouts or other poor getting started experiences. 
* **Non-Kubernetes**: Configure [health checks]({{< versioned_link_path fromRoot="/guides/traffic_management/request_processing/health_checks" >}}) on Envoy. Then, configure your load balancer to leverage these health checks, so that requests stop going to Envoy when it begins draining connections.

{{% notice tip %}}
Try a [Canary upgrade]({{< versioned_link_path fromRoot="/operations/upgrading/canary" >}}) to make sure that the newer version works as you expect before upgrading.
{{% /notice %}}

## Step 2: Review version {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}} changes

Prepare to upgrade by reviewing information about the version {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}}.

1. Check the changelogs for the type of Gloo Edge deployment that you have. Focus especially on any **Breaking Changes** that might require a different upgrade procedure. For Gloo Edge Enterprise, you might also review the open source changelogs because most of the proto definitions are open source.{{% notice tip %}}You can use the changelogs' built-in [comparison tool]({{< versioned_link_path fromRoot="/reference/changelog/open_source/#compareversions" >}}) to compare between your current version and the version that you want to upgrade to.{{% /notice %}}
   * [Open Source changelogs]({{% versioned_link_path fromRoot="/reference/changelog/open_source/" %}})
   * [Enterprise changelogs]({{% versioned_link_path fromRoot="/reference/changelog/enterprise/" %}}): Keep in mind that Gloo Edge Enterprise pulls in Gloo Edge Open Source as a dependency. Although the major and minor version numbers are the same for open source and enterprise, their patch versions often differ. For example, open source might use version `x.y.a` but enterprise uses version `x.y.b`. If you are unfamiliar with these versioning concepts, see [Semantic versioning](https://semver.org/). Because of the differing patch versions, you might notice different output when checking your version with `glooctl version`. For example, your API server might run Gloo Edge Enterprise version {{< readfile file="static/content/version_gee_latest.md" markdown="true">}}, which pulls in Gloo Edge Open Source version {{< readfile file="static/content/version_geoss_latest.md" markdown="true">}} as a dependency.
     ```bash
     ~ > glooctl version
     Client: {"version":"{{< readfile file="static/content/version_geoss_latest.md" markdown="true">}}"}
     Server: {"type":"Gateway","enterprise":true,"kubernetes":...,{"Tag":"{{< readfile file="static/content/version_gee_latest.md" markdown="true">}}","Name":"grpcserver-ee","Registry":"quay.io/solo-io"},...,{"Tag":"{{< readfile file="static/content/version_geoss_latest.md" markdown="true">}}","Name":"discovery","Registry":"quay.io/solo-io"},...}
     ```

2. Review the following main feature, Helm, CRD, and CLI changes for version {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}}.

3. If you still aren't sure about the version upgrade impact, scan our [Frequently-asked questions]({{% versioned_link_path fromRoot="/operations/upgrading/faq/" %}}). Also, feel free to post in the `#gloo` or `#gloo-enterprise` channels of our [public Slack](https://slack.solo.io/) if your use case doesn't quite fit the standard upgrade path.

### Feature changes {#features}

**New or improved features**:


**Deprecated features**:


**Removed features**:


### Helm changes {#helm}

**New Helm fields**:


**Deprecated Helm fields**:


**Removed Helm fields**:


### CRD changes {#crd}

**New and updated CRDs**:


**Deprecated CRDs**:


**Removed CRDs**:


### CLI changes {#cli}

**New CLI commands or options**:


**Changed behavior**:


## Step 2: Upgrade glooctl

Install or upgrade `glooctl`. When you upgrade, specify the Gloo Edge OSS version that corresponds to the Gloo Edge Enterprise version you want to upgrade to. To find the OSS version that corresponds to each Gloo Edge Enterprise release, see the [Gloo Edge Enterprise changelogs]({{% versioned_link_path fromRoot="/reference/changelog/enterprise/" %}}).

{{% notice warning %}}
Because `glooctl` can create resources in your cluster, such as with commands like `glooctl add route`, you might have errors in Gloo Edge if you create resources with an older version of `glooctl`.
{{% /notice %}}

You can upgrade `glooctl` in the following ways:
* [Use `glooctl upgrade`](#glooctl-upgrade)
* [Download a `glooctl` release](#download-a-glooctl-release)

### glooctl upgrade

You can use the `glooctl upgrade` command to download the latest binary. For more options, run `glooctl upgrade --help`. For example, you might use the `--release` flag, which can be useful to control which version you run.

1. Review the client and server versions of `glooctl`. 
   ```bash
   glooctl version
   ```
   Example output: Notice that the the client version is the same as the server components.
   ```bash
   Client: {"version":"{{< readfile file="static/content/version_geoss_n-1.md" markdown="true">}}"}
   Server: {"type":"Gateway","kubernetes":{"containers":[{"Tag":"{{< readfile file="static/content/version_geoss_n-1.md" markdown="true">}}","Name":"discovery","Registry":"quay.io/solo-io"},{"Tag":"{{< readfile file="static/content/version_geoss_n-1.md" markdown="true">}}","Name":"gateway","Registry":"quay.io/solo-io"},{"Tag":"{{< readfile file="static/content/version_geoss_n-1.md" markdown="true">}}","Name":"gloo-envoy-wrapper","Registry":"quay.io/solo-io"},{"Tag":"{{< readfile file="static/content/version_geoss_n-1.md" markdown="true">}}","Name":"gloo","Registry":"quay.io/solo-io"}],"namespace":"gloo-system"}}
   ```

2. Upgrade your version of `glooctl`.
   ```bash
   glooctl upgrade --release v{{< readfile file="static/content/version_geoss_latest.md" markdown="true">}}
   ```
   Example output:
   ```bash
   downloading glooctl-darwin-amd64 from release tag v{{< readfile file="static/content/version_geoss_latest.md" markdown="true">}}
   successfully downloaded and installed glooctl version v{{< readfile file="static/content/version_geoss_latest.md" markdown="true">}} to /usr/local/bin/glooctl
   ```

3. Confirm that the version is upgraded.
   ```bash
   glooctl version
   ```
   Example output: Notice that the client version is now {{< readfile file="static/content/version_geoss_latest.md" markdown="true">}}.
   ```bash
   Client: {"version":"{{< readfile file="static/content/version_geoss_latest.md" markdown="true">}}"}
   Server: {"type":"Gateway","kubernetes":{"containers":[{"Tag":"{{< readfile file="static/content/version_geoss_n-1.md" markdown="true">}}","Name":"discovery","Registry":"quay.io/solo-io"},{"Tag":"{{< readfile file="static/content/version_geoss_n-1.md" markdown="true">}}","Name":"gateway","Registry":"quay.io/solo-io"},{"Tag":"{{< readfile file="static/content/version_geoss_n-1.md" markdown="true">}}","Name":"gloo-envoy-wrapper","Registry":"quay.io/solo-io"},{"Tag":"{{< readfile file="static/content/version_geoss_n-1.md" markdown="true">}}","Name":"gloo","Registry":"quay.io/solo-io"}],"namespace":"gloo-system"}}
   ```

4. Check that your Gloo Edge components are **OK**. If `glooctl check` reports a problem, Gloo Edge might not work properly or Envoy might not get the updated configuration.
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

### Download a glooctl release

1. In your browser, navigate to the [Gloo project releases](https://github.com/solo-io/gloo/releases).
2. Click the version of `glooctl` that you want to install.
3. In the **Assets**, download the `glooctl` package that matches your operating system, and follow your operating system procedures for replacing your existing `glooctl` binary file with the upgraded version.

## Step 3: Apply minor version-specific changes

Each minor version might add custom resource definitions (CRDs) or otherwise have changes that Helm upgrades cannot handle seamlessly.

{{% notice warning %}}
New CRDs are automatically applied to your cluster when performing a `helm install` operation. However, they are not applied when performing an `helm upgrade` operation. This is a [deliberate design choice](https://helm.sh/docs/topics/charts/#limitations-on-crds) on the part of the Helm maintainers, given the risk associated with changing CRDs. Given this limitation, you must apply new CRDs to the cluster before upgrading.
{{% /notice %}}

1. **CRDs**: Check the [CRD changes](#crd) to see which CRDs are new, deprecated, or removed in version {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}}. <!--If applicable, add a first step to remove any CRDs that are removed in the upgrade version-->
   1. Apply the new and updated CRDs.
      {{< tabs >}}
      {{% tab name="Open Source" %}}
      ```sh
      helm repo update
      helm pull gloo/gloo --version {{< readfile file="static/content/version_geoss_latest.md" markdown="true">}} --untar
      kubectl apply -f gloo/crds
      ```
      {{% /tab %}}
      {{% tab name="Enterprise" %}}
      ```sh
      helm repo update
      helm pull glooe/gloo-ee --version {{< readfile file="static/content/version_gee_latest.md" markdown="true">}} --untar
      kubectl apply -f gloo-ee/charts/gloo/crds
      # If Gloo Federation is enabled
      kubectl apply -f gloo-ee/charts/gloo-fed/crds
      ```
      {{% /tab %}}
      {{< /tabs >}}
   2. Verify that the deployed CRDs use the same version as your current Gloo Edge installation.
      ```
      glooctl check-crds
      ```

2. **Feature changes**: Check the [Feature changes](#features) to see whether there are breaking changes you must address in your resources before you upgrade to {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}}. <!--If applicable, add steps to walk users though updating crs for any breaking changes-->

3. **Helm changes**: Check the [Helm changes](#features) to see whether there are new, deprecated, or removed Helm settings before you upgrade to {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}}.
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
   2. Edit the Helm values file or prepare the `--set` flags to make any changes that you want. If you do not want to use certain settings, comment them out.

## Step 4: Upgrade Gloo Edge

The following example upgrade process assumes that Gloo Edge is installed with Helm in a Kubernetes cluster and uses the Kubernetes load balancer.

{{% notice warning %}}
Using Helm 2 is not supported in Gloo Edge.
{{% /notice %}}

{{% notice note %}}
The upgrade creates a Kubernetes Job named `gateway-certgen` to generate a certificate for the validation webhook. The job
contains the `ttlSecondsAfterFinished` value so that the cluster cleans up the job automatically, but because this setting is still in
Alpha, your cluster might ignore this value. In this case, you might have an issue while upgrading in which the
upgrade attempts to change the `gateway-certgen` job, but the change fails because the job is immutable. To fix this issue,
you can delete the job, which already completed, and re-apply the upgrade.
{{% /notice %}}

### Upgrade steps

The following steps assume that you already installed Gloo Edge as a Helm release in the `gloo-system` namespace, and have set the Kubernetes context to the cluster.

1. Upgrade the Helm release. Include your installation values in a Helm values file (such as `-f values.yaml`) or in `--set` flags.
   {{< tabs >}}
   {{% tab name="Open Source" %}}
   ```shell script
   helm repo update
   helm upgrade -n gloo-system gloo gloo/gloo --version=v{{< readfile file="static/content/version_geoss_latest.md" markdown="true">}} \
   -f values.yaml
   ```

   Example output:
   ```
   Release "gloo" has been upgraded. Happy Helming!
   NAME: gloo
   LAST DEPLOYED: Thu Dec 12 12:22:16 2019
   NAMESPACE: gloo-system
   STATUS: deployed
   REVISION: 2
   TEST SUITE: None
   ```
   {{% /tab %}}
   {{% tab name="Enterprise" %}}
   Note that you must set your license key by using the `--set license_key=$license` flag or including the `license_key: $LICENSE-KEY` setting in your values file. If you do not have a license key, [request a Gloo Edge Enterprise trial](https://www.solo.io/gloo-trial).
   ```shell script
   helm repo update
   helm upgrade -n gloo-system gloo glooe/gloo-ee \
   --version=v{{< readfile file="static/content/version_gee_latest.md" markdown="true">}} \
   -f values.yaml \
   --set license_key=$license
   ```

   Example output:
   ```
   Release "glooe" has been upgraded. Happy Helming!
   NAME: glooe
   LAST DEPLOYED: Thu Dec 12 12:22:16 2019
   NAMESPACE: gloo-system
   STATUS: deployed
   REVISION: 2
   TEST SUITE: None
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

5. Now that your upgrade is complete, you can enable any [new features](#features) that you want to use.