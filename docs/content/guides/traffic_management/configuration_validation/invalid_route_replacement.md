---
menuTitle: Processing Partially Valid Config
title: Replace Invalid Routes
weight: 20
description: (Kubernetes Only) Gloo Edge can be configured to validate configuration before it is applied to the cluster. With validation enabled, any attempt to apply invalid configuration to the cluster will be rejected.
---

# Motivation

When a Virtual Service (or one of its delegated route tables) contains invalid configuration, 
its routes will no longer be propagated to the {{< protobuf name="gloo.solo.io.Proxy" display="Proxy">}}.
Instead, the last valid configuration for that Virtual Service will be used.

This behavior is used in order to ensure that invalid configuration does not lead to service outages.

In some cases, it may be desirable to update a virtual service even if its config becomes partially invalid. 
This is particularly useful when [delegating to Route Tables]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_types/delegation/">}}) as it ensures that a single Route Table will not block updates for other Route Tables which share the same Virtual Service. 

For this reason, Gloo Edge supports the ability to enable *automatic replacement of invalid routes* (including routes which point to a missing **Upstream**
or **UpstreamGroup** or routes that Gloo could not successfully process). 

This document demonstrates how to enable and use this feature. 

# Prerequisites

Make sure before starting you have:

- [Installed Gloo Edge in Gateway Mode]({{< versioned_link_path fromRoot="/installation/gateway/kubernetes">}})
- [Deployed the Petstore example App]({{< versioned_link_path fromRoot="/guides/traffic_management/hello_world/#deploy-the-pet-store-application">}})

# Create a Partially Valid Virtual Service 

Gloo Edge can be configured to admit partially invalid config by enabling *invalid route replacement*. The options for this behavior live on the {{< protobuf name="gloo.solo.io.Settings" display="Settings">}} resource.

Consider the following Virtual Service:

```yaml
{{< readfile file="guides/traffic_management/configuration_validation/partially_invalid_vs.yaml">}}
```

The route `/good-route` points to a valid destination (assuming the [`petstore` app has been deployed to the cluster]({{< versioned_link_path fromRoot="/guides/traffic_management/hello_world/">}}), while `/bad-route` points to an invalid destination.

Let's try applying this configuration to the cluster before enabling route replacement:

```yaml
kubectl apply -f - <<EOF
{{< readfile file="guides/traffic_management/configuration_validation/partially_invalid_vs.yaml">}}
EOF
```

If we check the status of our Virtual Service, we should see that it has a *Warning* status:

```bash
kubectl get vs -n default partially-valid -o yaml
```

```
apiVersion: gateway.solo.io/v1
kind: VirtualService
# ...
status:
  reason: "warning: \n  Route Warning: InvalidDestinationWarning. Reason: *v1.Upstream
    {kube-svc:anywhere-does-not-exist-1234 anywhere} not found"
  reportedBy: gateway
  state: 3
```

With route replacement disabled, the virtual service will not be propagated to the proxy. We can try by testing the good route:

```bash
curl $(glooctl proxy url)/good-route
```

The route will not be accepted and we'll see a `Connection Refused` error (if Gloo Edge had no prior config):

```noop
curl: (7) Failed to connect to 36.190.183.55 port 80: Connection refused
```

Let's see what happens when we enable route replacement.

# Enable Route Replacement

Enabling route replacement can be done by directly patching the **Settings** CRD, or modifying a Helm value:

{{< tabs >}}
{{< tab name="patching Settings" codelang="shell">}}
kubectl patch settings -n gloo-system default --patch '{"spec": {"gloo": {"invalidConfigPolicy": {"replaceInvalidRoutes": true, "invalidRouteResponseCode": 404, "invalidRouteResponseBody": "Gloo Gateway has invalid configuration. Administrators should run glooctl check to find and fix config errors."}}}}' --type=merge
{{< /tab >}}
{{< tab name="using Helm" codelang="bash">}}
# set the following in the helm overrides file
settings:
  replaceInvalidRoutes: true
  invalidConfigPolicy:
    invalidRouteResponseBody: Gloo Edge has invalid configuration. Administrators
      should run `glooctl check` to find and fix config errors.
    invalidRouteResponseCode: 404
    replaceInvalidRoutes: true
{{< /tab >}}
{{< /tabs >}}

Once this is done, the Settings should now show that `replaceInvalidRoutes` is set to `true`:

```bash
kubectl get settings -n gloo-system default -oyaml
```

{{< highlight yaml "hl_lines=13-16" >}}
apiVersion: gloo.solo.io/v1
kind: Settings
# ...
spec:
  devMode: true
  discoveryNamespace: gloo-system
  gateway:
    validation:
      alwaysAccept: true
      proxyValidationServerAddr: gloo:9988
  gloo:
    invalidConfigPolicy:
      invalidRouteResponseBody: Gloo Edge has invalid configuration. Administrators
        should run `glooctl check` to find and fix config errors.
      invalidRouteResponseCode: 404
      replaceInvalidRoutes: true
    xdsBindAddr: 0.0.0.0:9977
  kubernetesArtifactSource: {}
  kubernetesConfigSource: {}
  kubernetesSecretSource: {}
  refreshRate: 60s
{{< /highlight >}}


{{% notice note %}}
`invalidRouteResponseCode` and `invalidRouteResponseBody` can be modified to customize the 
response code and body returned to clients that hit invalid routes.
{{% /notice %}}

If we try the good route again:

```bash
curl $(glooctl proxy url)/good-route
```

The route will have been accepted and is now being served by the proxy:

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

We can also see how the bad route behaves when it is replaced:

```bash
curl $(glooctl proxy url)/bad-route
```

The route will return the status code and body defined in the Settings:

```
Gloo Edge has invalid configuration. Administrators should run `glooctl check` to find and fix config errors.
```

Great! We've just seen the benefits of enabling route replacement on our virtual services. 

Note that, when using route replacement, deleting an Upstream/Service object which has active routes pointing to it will cause those routes to fail. When enabling route replacement, be certain that this behavior is preferable to the default (halting configuration updates to the proxy). 

We appreciate questions and feedback on Gloo Edge validation or any other feature on [the solo.io slack channel](https://slack.solo.io/) as well as our [GitHub issues page](https://github.com/solo-io/gloo).

