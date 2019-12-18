---
menuTitle: Configuration Validation
title: Config Reporting & Validation
weight: 60
description: (Kubernetes Only) Gloo can be configured to validate configuration before it is applied to the cluster. With validation enabled, any attempt to apply invalid configuration to the cluster will be rejected.
---

## Motivation

When configuring an API gateway or edge proxy, invalid configuration can quickly lead to bugs, service outages, and 
security vulnerabilities. 

This document explains features in Gloo designed to prevent invalid configuration from propagating to the 
data plane (the Gateway Proxies).

## How Gloo Validates Configuration

Validation in Gloo is comprised of a two step process:

1. First, resources are admitted (or rejected) via a [Kubernetes Validating Admission Webhook](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/). Configuration options for the webhook live
in the `settings.gloo.solo.io` custom resource.

2. Once a resource is admitted, Gloo processes it in a batch with all other admitted resources. If any errors are detected 
in the admitted objects, Gloo will report the errors on the `status` of those objects. At this point, Envoy configuration will 
not be updated until the errors are resolved.


Each *Proxy* gets its own configuration; if config for an individual proxy is invalid, it does not affect the other proxies.
The proxy that *Gateways* and their *Virtual Services* will be applied to can be configured via the `proxyNames` option on 
  the {{< protobuf name="gateway.solo.io.v2.Gateway" display="Gateway resource">}}.

{{% notice note %}}

- You can run `glooctl check` locally to easily spot any configuration errors on resources that have been admitted to your cluster.

{{% /notice %}}

## Warnings and Errors

Gloo processes an admitted config resource, it can report one of three status types on the resource:

- *Accepted*: The resource has been accepted and applied to the system.
- *Rejected*: The resource has invalid configuration and has not been applied to the system.
- *Warning*: The resource has valid config but points to a missing/misconfigured resource.

When a resource is in *Rejected* or *Warning* state, its configuration is not propagated to the proxy.

## Using the Validating Webhook

Admission Validation provides a safeguard to ensure Gloo does not halt processing of configuration. If a resource 
would be written or modified in such a way to cause Gloo to report an error, it is instead rejected by the Kubernetes 
API Server before it is written to persistent storage.

Gloo runs a [Kubernetes Validating Admission Webhook](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/)
which is invoked whenever a `gateway.solo.io` custom resource is created or modified. This includes 
{{< protobuf name="gateway.solo.io.v2.Gateway" display="Gateways">}}, 
{{< protobuf name="gateway.solo.io.VirtualService" display="Virtual Services">}}.),
and {{< protobuf name="gateway.solo.io.RouteTable" display="Route Tables">}}.

The [validating webhook configuration](https://github.com/solo-io/gloo/blob/master/install/helm/gloo/templates/5-gateway-validation-webhook-configuration.yaml) is enabled by default by Gloo's Helm chart and `glooctl install gateway`. This admission webhook can be disabled 
by removing the `ValidatingWebhookConfiguration`.

The webhook can be configured to perform strict or permissive validation, depending on the `gateway.validation.alwaysAccept` setting in the 
{{< protobuf name="gloo.solo.io.Settings" display="Settings">}} resource.

When `alwaysAccept` is `true` (currently the default is `true`), resources will only be rejected when Gloo fails to 
deserialize them (due to invalid JSON/YAML).

To enable "strict" admission control (rejection of resources with invalid config), set `alwaysAccept` to false.

When strict admission control is enabled, any resource that would produce a `Rejected` status will be rejected on admission.
Resources that would produce a `Warning` status are still admitted.

## Enabling Strict Validation Webhook 
 
 
By default, the Validation Webhook only logs the validation result, but always admits resources with valid YAML (even if the 
configuration options are inconsistent/invalid).

The webhook can be configured to reject invalid resources via the 
{{< protobuf name="gloo.solo.io.Settings" display="Settings">}} resource.

If using Helm to manage settings, set the following value:

```bash
--set gateway.validation.alwaysAcceptResources=false
```

If writing Settings directly to Kubernetes, add the following to the `spec.gateway` block:

{{< highlight yaml "hl_lines=13-15" >}}
apiVersion: gloo.solo.io/v1
kind: Settings
metadata:
  annotations:
    helm.sh/hook: pre-install
    helm.sh/hook-weight: "5"
  labels:
    app: gloo
  name: default
  namespace: gloo-system
spec:
  discoveryNamespace: gloo-system
  gateway:
    validation:
      alwaysAccept: false
  gloo:
    xdsBindAddr: 0.0.0.0:9977
  kubernetesArtifactSource: {}
  kubernetesConfigSource: {}
  kubernetesSecretSource: {}
  refreshRate: 60s
{{< /highlight >}}

Once these are applied to the cluster, we can test that validation is enabled:


```bash
kubectl apply -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: reject-me
  namespace: default
spec:
  virtualHost:
    routes:
      # this route is missing a path specifier and will be rejected
      - matcher: {}
        routeAction:
          single:
            upstream:
              name: does-not-exist
              namespace: anywhere
EOF

```

We should see the request was rejected:

```bash
Error from server: error when creating "STDIN": admission webhook "gateway.gloo-system.svc" denied the request: resource incompatible with current Gloo snapshot: [Route Error: InvalidMatcherError. Reason: no path specifier provided]
```

Great! Validation is working, providing us a quick feedback mechanism and preventing Gloo from receiving invalid config. 

We appreciate questions and feedback on Gloo validation or any other feature on [the solo.io slack channel](https://slack.solo.io/) as well as our [GitHub issues page](https://github.com/solo-io/gloo).


