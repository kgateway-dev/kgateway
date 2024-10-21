---
title: Prepare to upgrade
weight: 10
description: Prepare your environment, review version changes, and review FAQs before you upgrade Gloo Gateway.
---

Before you upgrade Gloo Gateway, complete the following preparatory steps:
* [Prepare your environment](#prepare), such as upgrading your current version to the latest patch and upgrading any dependencies to the required supported versions. 
* [Review important changes](#review-changes) made to Gloo Gateway in version {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}}, including CRD, Helm, CLI, and feature changes.
* [Review frequently-asked questions](#faqs) about the upgrade process.

## Prepare your environment {#prepare}

Review the following preparatory steps that might be required for your environment.

### Upgrade your current minor version to the latest patch {#current-patch}

Before you upgrade your minor version, first upgrade your current version to the latest patch. For example, if you currently run Gloo Gateway Enterprise version {{< readfile file="static/content/version_gee_n-1_oldpatch.md" markdown="true">}}, first upgrade your installation to version {{< readfile file="static/content/version_gee_n-1.md" markdown="true">}}. This ensures that your current environment is up-to-date with any bug fixes or security patches before you begin the minor version upgrade process.

1. Find the latest patch of your minor version by checking the [Open Source changelog]({{% versioned_link_path fromRoot="/reference/changelog/open_source/" %}}) or [Enterprise changelog]({{% versioned_link_path fromRoot="/reference/changelog/enterprise/" %}}).
2. Go to the documentation set for your current minor version. For example, if you currently run Gloo Gateway Enterprise version {{< readfile file="static/content/version_gee_n-1_oldpatch.md" markdown="true">}}, use the drop-down menu in the header of this page to select **v{{< readfile file="static/content/version_geoss_n-1_minor.md" markdown="true">}}.x**.
3. Follow the upgrade guide, using the latest patch for your minor version.

### If required, perform incremental minor version updates {#minor-increment}

If you plan to upgrade to a version that is more than one minor version greater than your current version, such as to version {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}} from {{< readfile file="static/content/version_geoss_n-2_minor.md" markdown="true">}} or older, you must upgrade incrementally. For example, you must first use the upgrade guide in the v{{< readfile file="static/content/version_geoss_n-1_minor.md" markdown="true">}}.x documentation set to upgrade from {{< readfile file="static/content/version_geoss_n-2_minor.md" markdown="true">}} to {{< readfile file="static/content/version_geoss_n-1_minor.md" markdown="true">}}, and then follow the upgrade guide in the v{{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}}.x documentation set to upgrade from {{< readfile file="static/content/version_geoss_n-1_minor.md" markdown="true">}} to {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}}.

### Upgrade dependencies {#dependencies}

Check that your underlying infrastructure platform, such as Kubernetes, and other dependencies run a version that is supported for {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}}.

1. Review the [supported versions]({{% versioned_link_path fromRoot="/reference/support/#supported-versions" %}}) for dependencies such as Kubernetes, Istio, Helm, and more.
2. Compare the supported versions against the versions you currently use.
3. If necessary, upgrade your dependencies, such as consulting your cluster infrastructure provider to upgrade the version of Kubernetes that your cluster runs.

### Consider settings to avoid downtime {#downtime}

You might deploy Gloo Gateway in Kubernetes environments that use the Kubernetes load balancer, or in non-Kubernetes environments. Depending on your setup, you can take additional steps to avoid downtime during the upgrade process.

* **Kubernetes**: Enable [Envoy readiness and liveness probes]({{< versioned_link_path fromRoot="/operations/production_deployment/#enable-health-checks" >}}) during the upgrade. When these probes are set, Kubernetes sends requests only to the healthy Envoy proxy during the upgrade process, which helps to prevent potential downtime. The probes are not enabled in default installations because they can lead to timeouts or other poor getting started experiences. 
* **Non-Kubernetes**: Configure [health checks]({{< versioned_link_path fromRoot="/guides/traffic_management/request_processing/health_checks" >}}) on Envoy. Then, configure your load balancer to leverage these health checks, so that requests stop going to Envoy when it begins draining connections.

## Review version {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}} changes {#review-changes}

Review the following changes made to Gloo Gateway in version {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}}. For some changes, you might be required to complete additional steps during the upgrade process.

### Breaking changes

**Envoy version 1.31 upgrade**

The Envoy dependency in Gloo Gateway 1.18 was upgraded from 1.29.x to 1.31.x. This upgrade includes the following changes. For more information about these changes, see the [Envoy changelog documentation](https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.31/v1.31).
* **Opencensus**: Opencensus was marked as deprecated in previous Envoy releases. Starting in the 1.31.x Envoy release, Opencensus is now disabled by default. If Opencensus is set, Envoy rejects the configuration. You can use Envoy's [layered_runtime](https://github.com/envoyproxy/envoy/blob/38530270d6cb3a3a71a9b70b3de55854750b75a9/configs/using_deprecated_config.yaml) section to enable deprecated configuration so that you can continue using Opencensus. However, note that Opencensus is completely removed in Envoy version 1.32.x.
* **JWT tokens**: The behavior for extracting JWT tokens changed in the 1.29.x Envoy release. Previously, the JWT token was cut into non-base64 characters. Now, the entire JWT token is passed for validation. You can no longer revert this change by setting `envoy.reloadable_features.token_passed_entirely` to `false` as this option was removed in the 1.31.x Envoy release.
* **JWT_authn**: Provider URIs that are defined in the `jwt_authn` section are now validated for RFC-compliance. Envoy might fail to start correctly if non-compliant URIs are found. If the URI validation is too strict, you can temporarily disable it by setting the runtime guard `envoy.reloadable_features.jwt_authn_validate_uri` to false. Common URI issues that were previously ignored, include: 
    - Hostname contains `_` (underscore character)
    - URL contains non-English characters (ASCII code > 127)
    - URL contains an unencoded ` ` (space character)
    - URL contains TAB (ASCII code 9) or FormFeed (ASCII code 12) characters
* **JWT_authn**: The provider [forward](https://www.envoyproxy.io/docs/envoy/v1.31.2/api-v3/extensions/filters/http/jwt_authn/v3/config.proto#envoy-v3-api-field-extensions-filters-http-jwt-authn-v3-jwtprovider-forward) configuration changed. Previously, JWTs could only be removed from headers. Starting in Envoy version 1.31.x, JWTs can now be removed from query parameters. You can temporarily revert this change by setting `envoy.reloadable_features.jwt_authn_remove_jwt_from_query_params` to `false`.
* **access_log**: The following access log format specifiers changed: 
    - The upstream connection address is now used for the `%UPSTREAM_REMOTE_ADDRESS%`, `%UPSTREAM_REMOTE_PORT%` and `%UPSTREAM_REMOTE_ADDRESS_WITHOUT_PORT%` access log format specifiers. Previously, the upstream host address was used. You can temporarily revert this change by setting the runtime guard `envoy.reloadable_features.upstream_remote_address_use_connection` to `false`.
    - The `%UPSTREAM_CLUSTER_RAW%` access log formatter was added to log the original upstream cluster name, regardless of whether `alt_stat_name` is set.
    - SNIs are automatically sanitized for potential log injection. The invalid characters are replaced by `_` with an `invalid:` marker. To disable this feature, set `envoy.reloadable_features.sanitize_sni_in_access_log` is set to `false`.
* **YAML parsing**: The behavior for parsing YAML configuration changed. Previously, malformed boolean values and fraction objects that set `true` or `false` as a string value, are no longer interpreted as a boolean value. You can revert this change by setting `envoy.reloadable_features.reject_invalid_yaml` to `false`.
* **HTTP/2**: HTTP/2 colon prefixed headers are now sanitized by Envoy. Previously, sanitation was performed by the `nghttp2` library, which caused pseudo headers with upper case letters to fail validation. Now, these pseudo headers pass validation. You can temporarily revert this change by setting the runtime guard `envoy.reloadable_features.sanitize_http2_headers_without_nghttp2` to `false`. 
* **Local ratelimit**: The token bucket implementation changed. Previously, a timer-based token bucket was used to assign tokens to connections. In Envoy 1.31.x, the new AtomicToken bucket is used that is no longer timer-based. Tokens are now automatically refilled when the token bucket is accessed. Because of this change, the `x-ratelimit-reset` header is no longer sent. You can temporarily revert this change by setting the runtime guard `envoy.reloadable_features.no_timer_based_rate_limit_token_bucket` to `false`.


<!-- ggv2-related changes:
ggv2 - Disable Istio Envoy proxy from running by default and only rely on proxyless Istio agent mtls integration. Note: Although this is a change to the default behavior of the istio integration, this should not have any impact on most users as the sidecar proxy was unused in the data path. (https://github.com/solo-io/solo-projects/issues/5711)

ggv2 - glooctl get proxy will not work if you have persisted Proxy CRs in etcD and you are querying and older server version (1.16 and below). In general, we recommend that you keep your client and server versions in sync. You can verify the client/server versions you are currently running by calling glooctl version. (https://github.com/solo-io/gloo/pull/9226)
-->

### Changelogs

Check the changelogs for the type of Gloo Gateway deployment that you have. Focus especially on any **Breaking Changes** that might require a different upgrade procedure. For Gloo Gateway Enterprise, you might also review the open source changelogs because most of the proto definitions are open source.
* [Open Source changelogs]({{% versioned_link_path fromRoot="/reference/changelog/open_source/" %}})
* [Enterprise changelogs]({{% versioned_link_path fromRoot="/reference/changelog/enterprise/" %}}): Keep in mind that Gloo Gateway Enterprise pulls in Gloo Gateway Open Source as a dependency. Although the major and minor version numbers are the same for open source and enterprise, their patch versions often differ. For example, open source might use version `x.y.a` but enterprise uses version `x.y.b`. If you are unfamiliar with these versioning concepts, see [Semantic versioning](https://semver.org/). Because of the differing patch versions, you might notice different output when checking your version with `glooctl version`. For example, your API server might run Gloo Gateway Enterprise version {{< readfile file="static/content/version_gee_latest.md" markdown="true">}}, which pulls in Gloo Gateway Open Source version {{< readfile file="static/content/version_geoss_latest.md" markdown="true">}} as a dependency.
  ```bash
  ~ > glooctl version
  Client: {"version":"{{< readfile file="static/content/version_geoss_latest.md" markdown="true">}}"}
  Server: {"type":"Gateway","enterprise":true,"kubernetes":...,{"Tag":"{{< readfile file="static/content/version_gee_latest.md" markdown="true">}}","Name":"grpcserver-ee","Registry":"quay.io/solo-io"},...,{"Tag":"{{< readfile file="static/content/version_geoss_latest.md" markdown="true">}}","Name":"discovery","Registry":"quay.io/solo-io"},...}
  ```

{{% notice tip %}}
You can use the changelogs' built-in [comparison tool]({{< versioned_link_path fromRoot="/reference/changelog/open_source/#compareversions" >}}) to compare between your current version and the version that you want to upgrade to.
{{% /notice %}}

### Feature changes {#features}

Review the following summary of important new, deprecated, or removed features.

{{% notice note %}}
The following lists consist of the changes that were initially introduced with the {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}}.0 release. These changes might be backported to earlier versions of Gloo Gateway. Additionally, there might be other changes that are introduced in later {{< readfile file="static/content/version_geoss_latest_minor.md" markdown="true">}} patch releases. For patch release changes, check the [changelogs](#changelogs).
{{% /notice %}}

**New or improved features**:

* **Apply JWT policy at the route-level**: Now, you can apply JWT policies to specific routes by configuring the `jwtProvidersStaged` settings in the route option. Previously, JWT policies applied at the gateway level and were configured in only the VirtualHost option. With this new feature, you can apply JWT policies at both the route and gateway level. For more information and example steps, see [Route-level JWT policy]({{< versioned_link_path fromRoot="/security/auth/jwt/route-jwt-policy/" >}}).

<!--
**Deprecated features**:
N/A

**Removed features**:
N/A
-->

<!-- ggv2-related changes:
ggv2 - Added support for settings.gloo.istioOptions.enableAutoMtls to implement auto mTLS via Envoy transportsocketmatch. (https://github.com/solo-io/solo-projects/issues/5695)

ggv2 - Support policy attachment for RouteOption resources (https://github.com/solo-io/solo-projects/issues/5714)

ggv2 - Add new GatewayParameters CRD to allow configuration of dynamically provisioned proxies in Gloo Gateway. (https://github.com/solo-io/solo-projects/issues/5909)

ggv2 - RouteOption resources used for policy attachment (via targetRef) will now have their status correctly set based on the result of translation (https://github.com/solo-io/solo-projects/issues/5934)

ggv2 - Introduce targetRef field to VirtualHostOption resource. This will allow users of the Kubernetes Gateway API to specify which Gateway resource, and optionally which Listeners on that resource will be affected by the VirtualHostOption (https://github.com/solo-io/solo-projects/issues/6002)

ggv2 - Introduce VirtualHostOption plugin for the Kubernetes Gateway API integration. This plugin will honor VirtualHostOption resources and when translating K8s Gateway resources and apply their contents to the appropriate sections of the final proxy object. (https://github.com/solo-io/solo-projects/issues/6002)

ggv2 - Route delegation: explicitly pass route's hostnames to plugins so that delegatee (child) routes without hostnames can be associated with their corresponding hostnames. (https://github.com/solo-io/solo-projects/issues/6121)

ggv2 - Route delegation: enable HTTP route delegation with Gateway API, such that a parent route may delegate routing decisions to other routes that match the parent route rules consisting of path prefix, headers, and query parameters. (https://github.com/solo-io/solo-projects/issues/6121)

edge, yes (issue open for full docs) - Adds the API for a new enterprise only feature designed to allow authenticating requests using tokens from the google metadata service before sending the requests upstreams. This feature will be exposed as a new Upstream type. (https://github.com/solo-io/gloo/issues/6828)

ggv2 - Upstream Support: enable the use of Gloo Edge v1 Upstreams as destinations for using routes and mirror policy from the K8s Gateway API. (https://github.com/solo-io/solo-projects/issues/6129)

ggv2 - Add VirtualHostOptions status tracking for Kubernetes Gateways (https://github.com/solo-io/solo-projects/issues/6044)

ggv2 - This change implements policy inheritance, specifically Additionally, it does the following:
Refactors the RouteOption query API to perform merging
Translator tests for the many scenarios of policy inheritance.
Converts delegation translator test to a table-driven test.
E2e tests to verify the inheritance and merge functionality. (https://github.com/solo-io/solo-projects/issues/6161)

ggv2 - Adds webhook validation for Gloo Gateway Policies (e.g. RouteOption and VirtualHostOption) when used with Kubernetes Gateway API (https://github.com/solo-io/solo-projects/issues/6063)

ggv2 - Introduced a new default GatewayParameters which is associated with a GatewayClass and represents the default values applied to Gateways created from that GatewayClass that don't otherwise have a specific GatewayParameters attached. (https://github.com/solo-io/solo-projects/issues/6107)

ggv2 - gateway2: enable self-managed Gateways Adds capability to integrate self-managed gateways It adds a selfManaged field to the GatewayParameters

ggv2 - New CRDs added for ListenerOption and HttpListenerOption resources (https://github.com/solo-io/solo-projects/issues/5941)

ggv2 - Add ListenerOption as a policy resource for use with Kube Gateway API objects.

ggv2 - Add API for adding metadata to endpoints in static/failover upstreams. This metadata can

ggv2 - Add support for the envoy.http.stateful_session.header filter This support has been added via a new HTTPListener option, stateful_session which can be used to configure the filter. Envoy notes about this filter: - Stateful sessions can result in imbalanced load across upstreams and allow external actors to direct requests to specific upstream hosts. Operators should carefully consider the security and reliability implications of stateful sessions before enabling this feature. - This extension is functional but has not had substantial production burn time, use only with this caveat. - This extension has an unknown security posture and should only be used in deployments where both the downstream and upstream are trusted. (https://github.com/solo-io/gloo/issues/9104)

ggv2 - Enables routing to AWS Lambda and Azure Function upstreams via the GGv2 API. (https://github.com/solo-io/solo-projects/issues/6160)

ggv2 - dd HttpListenerOption policy for use with Kube Gateway API resources (https://github.com/solo-io/solo-projects/issues/6319)
-->

### Helm changes {#helm}

Review the following summary of important new, deprecated, or removed Helm fields. For full details, see the [changelogs](#changelogs).

**New Helm fields**:

**Updated Helm fields**:

**Deprecated Helm fields**:

<!--GGv2 changes:

ggv2 - Introduced new fields to kubeGateway top-level field which configure the deployed Gateway proxies generated from a Gateway. Also introduced a new default GatewayParameters to be rendered when kubeGateway.enabled=true. This contains defaults for Istio/SDS, as well as things like envoy image, deployment replicas, and extra labels in the pod template. (https://github.com/solo-io/solo-projects/issues/6107)

ggv2 - Add k8s Gateway Istio integration values to the Gloo Gateway Helm chart under kubeGateway.gatewayParameters.glooGateway. (https://github.com/solo-io/solo-projects/issues/5743)

ggv2 - Rename the kube gateway envoy container image helm value from kubeGateway.gatewayParameters.glooGateway.image to kubeGateway.gatewayParameters.glooGateway.envoyContainer.image. (https://github.com/solo-io/solo-projects/issues/6107)

ggv2 - Introduce gateway.validation.webhook.enablePolicyApi which controls whether or not RouteOptions and VirtualHostOptions CRs are subject to validation. By default, this value is true. The validation of these Policy APIs only runs if the Kubernetes Gateway integration is enabled (kubeGateway.enabled). (https://github.com/solo-io/solo-projects/issues/6352)
-->

### CRD changes {#crd}

New CRDs are automatically applied to your cluster when performing a `helm install` operation, but are _not_ applied when performing an `helm upgrade` operation. This is a [deliberate design choice](https://helm.sh/docs/topics/charts/#limitations-on-crds) on the part of the Helm maintainers, given the risk associated with changing CRDs. Given this limitation, you must apply new CRDs to the cluster before upgrading. 

Review the following summary of important new, deprecated, or removed CRD updates. For full details, see the [changelogs](#changelogs).

As part of the {{< readfile file="static/content/version_geoss_latest.md" markdown="true">}} release, no CLI changes were introduced.
<!--
**New and updated CRDs**:


**Deprecated CRDs**:
N/A

**Removed CRDs**:
N/A
-->

### CLI changes {#cli}

You must upgrade `glooctl` before you upgrade Gloo Gateway. Because `glooctl` can create resources in your cluster, such as with `glooctl add route`, you might have errors in Gloo Gateway if you create resources with an older version of `glooctl`.

As part of the {{< readfile file="static/content/version_geoss_latest.md" markdown="true">}} release, no CLI changes were introduced.
<!--
Review the following summary of important new, deprecated, or removed CLI options. For full details, see the [changelogs](#changelogs).

**New CLI commands or options**:

* `glooctl create secret encryptionkey`: [Create encryption secrets]({{% versioned_link_path fromRoot="/reference/cli/glooctl_create_secret_encryptionkey/" %}}), such as to use in the `cipherConfig` field of the `ExtAuthConfig` resource.

**Changed behavior**:-->


## Frequently-asked questions {#faqs}

Review the following frequently-asked questions about the upgrade process. If you still aren't sure about the version upgrade impact, or if your use case doesn't quite fit the standard upgrade path, feel free to post in the `#gloo` or `#gloo-enterprise` channels of our [public Slack](https://slack.solo.io/).

### How do I upgrade Gloo Gateway in testing or sandbox environments?

If downtime is not a concern for your use case, you can follow the [Quick upgrade guide]({{< versioned_link_path fromRoot="/operations/upgrading/upgrade_steps" >}}) to update your Gloo Gateway installation.

Note that for sandbox or exploratory environments, the easiest way to upgrade is to uninstall Gloo Gateway by running `glooctl uninstall --all`. Then, re-install Gloo Gateway at the desired version by the following one of the [installation guides]({{< versioned_link_path fromRoot="/installation" >}}).
 
### How do I upgrade Gloo Gateway in a production environment, where downtime is unacceptable?

The basic `helm upgrade` process is not suitable for environments in which downtime is unacceptable. Instead, you can follow the [Canary upgrade]({{% versioned_link_path fromRoot="/operations/upgrading/canary/" %}}) guide to deploy multiple version of Gloo Gateway to your cluster, and test the upgrade version before uninstalling the existing version.

Additionally, you might need to take steps to account for other factors such as Gloo Gateway version changes, probe configurations, and external infrastructure like the load balancer that Gloo Gateway uses. Consider setting up [liveness probes and healthchecks](#downtime) in your environment.

### What happens to my Gloo Gateway CRs during an upgrade? How do I handle breaking changes?

A typical upgrade of Gloo Gateway across minor versions should not cause disruptions to the existing Gloo Gateway state. In the case of a breaking change, Solo will communicate through the upgrade guides, changelogs, or other channels if you must make a specific adjustment to perform the upgrade. Note that you can always use the `glooctl debug yaml` command to download the current Gloo Gateway state to one large YAML manifest.

### Is the upgrade procedure different if I am not a cluster administrator?

If you are not an administrator of your cluster, you might be unable to create custom resource definitions (CRDs) and other cluster-scoped resources, such as cluster roles and cluster role bindings. If you encounter an error related to these resources, you can disable their creation by including the following setting in your Helm values:
```yaml
global:
  glooRbac:
    create: false
```

Otherwise, you can try performing an installation of Gloo Gateway that is scoped to a single namespace by including the following setting in your Helm values:
```yaml
global:
  glooRbac:
    namespaced: true
```

### Why do I get an error about re-creating CRDs when upgrading using `helm install` or `helm upgrade`?

Helm v2 does not manage CRDs well, and is not supported in Gloo Gateway. Upgrade to Helm v3, delete the CRDs, and try again.

### Why do I get an error about a `gateway-certgen` job?

The upgrade creates a Kubernetes Job named `gateway-certgen` to generate a certificate for the validation webhook. The job contains the `ttlSecondsAfterFinished` value so that the cluster cleans up the job automatically, but because this setting is still in Alpha, your cluster might ignore this value. In this case, you might have an issue while upgrading in which the upgrade attempts to change the `gateway-certgen` job, but the change fails because the job is immutable. To fix this issue, you can delete the job, which already completed, and re-apply the upgrade.
