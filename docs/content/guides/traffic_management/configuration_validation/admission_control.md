---
menuTitle: Admission Control
title: Admission Control
weight: 10
description: (Kubernetes Only) Gloo Edge can be configured to validate configuration before it is applied to the cluster. With validation enabled, any attempt to apply invalid configuration to the cluster will be rejected.
---

## Motivation

Gloo Edge can prevent invalid configuration from being written to Kubernetes with the use of a [Kubernetes Validating Admission Webhook](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/).

This document explains how to enable and configure admission control in Gloo Edge.

## Using the Validating Admission Webhook

Admission Validation provides a safeguard to ensure Gloo Edge does not halt processing of configuration. If a resource 
would be written or modified in such a way to cause Gloo Edge to report an error, it is instead rejected by the Kubernetes 
API Server before it is written to persistent storage.

Gloo Edge runs a [Kubernetes Validating Admission Webhook](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/)
which is invoked whenever a `gateway.solo.io` custom resource is created or modified. This includes
{{< protobuf name="gateway.solo.io.Gateway" display="Gateways">}},
{{< protobuf name="gateway.solo.io.VirtualService" display="Virtual Services">}},
and {{< protobuf name="gateway.solo.io.RouteTable" display="Route Tables">}}.

The [validating webhook configuration](https://github.com/solo-io/gloo/blob/main/install/helm/gloo/templates/5-gateway-validation-webhook-configuration.yaml) is enabled by default by Gloo Edge's Helm chart and `glooctl install gateway`. This admission webhook can be disabled 
by removing the `ValidatingWebhookConfiguration`.

The webhook can be configured to perform strict or permissive validation, depending on the `gateway.validation.alwaysAccept` setting in the 
{{< protobuf name="gloo.solo.io.Settings" display="Settings">}} resource.

When `alwaysAccept` is `true` (currently the default is `true`), resources will only be rejected when Gloo Edge fails to 
deserialize them (due to invalid JSON/YAML).

To enable "strict" admission control (rejection of resources with invalid config), set `alwaysAccept` to false.

When strict admission control is enabled, any resource that would produce a `Rejected` status will be rejected on admission.
Resources that would produce a `Warning` status are still admitted.

## Enabling Strict Validation Webhook 
 
 
By default, the Validation Webhook only logs the validation result, but always admits resources with valid YAML (even if the 
configuration options are inconsistent/invalid).

The webhook can be configured to reject invalid resources via the 
{{< protobuf name="gloo.solo.io.Settings" display="Settings">}} resource.

If using Helm to manage settings, set the following values:

```bash
--set gateway.validation.alwaysAcceptResources=false
--set gateway.validation.enabled=true
```

If writing Settings directly to Kubernetes, add the following to the `spec.gateway` block:

{{< highlight yaml "hl_lines=12-15" >}}
apiVersion: gloo.solo.io/v1
kind: Settings
metadata:
  labels:
    app: gloo
  name: default
  namespace: gloo-system
spec:
  discoveryNamespace: gloo-system
  gloo:
    xdsBindAddr: 0.0.0.0:9977
  gateway:
    validation:
      alwaysAcceptResources: false
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
  namespace: gloo-system
spec:
  virtualHost:
    routes:
    - matchers:
      - headers:
        - name: foo
          value: bar
      routeAction:
        single:
          upstream:
            name: does-not-exist
            namespace: gloo-system
EOF

```

We should see the request was rejected:

```noop
Error from server: error when creating "STDIN": admission webhook "gateway.gloo-system.svc" denied the request: resource incompatible with current Gloo Edge snapshot: [Route Error: InvalidMatcherError. Reason: no path specifier provided]
```

Great! Validation is working, providing us a quick feedback mechanism and preventing Gloo Edge from receiving invalid config.

Another way to use the validation webhook is via `kubectl apply --server-dry-run`, which allows users to test
configuration before attempting to apply it to their cluster.

We appreciate questions and feedback on Gloo Edge validation or any other feature on [the solo.io slack channel](https://slack.solo.io/) as well as our [GitHub issues page](https://github.com/solo-io/gloo).
