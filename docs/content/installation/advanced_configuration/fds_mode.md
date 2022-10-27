---
title: Configuring Function Discovery
weight: 10
description: Using automatic function discovery (ie, discovering and understanding Swagger/OAS docs or gRPC reflection)
---

Gloo Edge's **Function Discovery Service** (FDS) attempts to poll endpoints for:

* A path serving an [OpenAPI (Swagger) document](https://swagger.io/specification/).
* gRPC Services with [gRPC Reflection](https://github.com/grpc/grpc/blob/master/doc/server-reflection.md) enabled.


The default endpoints evaluated for `swagger` or `OpenAPISpec` docs are:

```
"/openapi.json"
"/swagger.json"
"/swagger/docs/v1"
"/swagger/docs/v2"
"/v1/swagger"
"/v2/swagger"
```

If you have an OpenAPI definition on a different endpoint, you can customize the location by configuring it in the `serviceSpec.rest.swaggerInfo.url` field. For example, for a given Upstream, you can add the following including an explicit location for the OpenAPI document:


{{< highlight yaml "hl_lines=17-20" >}}
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  labels:
    discovered_by: kubernetesplugin
    service: petstore
  name: default-petstore-8080
  namespace: gloo-system
spec:
  discoveryMetadata: {}
  kube:
    selector:
      app: petstore
    serviceName: petstore
    serviceNamespace: default
    servicePort: 8080
    serviceSpec:
      rest:
        swaggerInfo:
          url: http://petstore.default:8080/foo/bar/swagger.json

{{< /highlight >}}

{{% notice note %}}

Note, Function Discovery needs to be enabled for this to work. See the next sections.

{{% /notice %}}

## Function Discovery Service (FDS)

Using FDS means that the Gloo Edge `discovery` component will make HTTP requests to all `Upstreams` known to Gloo Edge trying to discover functions. This behavior causes increased network traffic and may be undesirable if it causes unexpected behavior or logs to appear in the services Gloo Edge is attempting to poll. For this reason, we may want to restrict the manner in which FDS polls services.

Gloo Edge allows whitelisting/blacklisting services, either by namespace or on the individual service level.

We can use these configuration settings to restrict FDS to discover only the namespaces or individual services we choose.

---

## Configuring the `fdsMode` Setting

FDS can run in one of 3 modes:

* `BLACKLIST`: The most liberal FDS polling policy. Using this mode, FDS will poll any service unless its namespace or the service itself is explicitly blacklisted.
* `WHITELIST`: A more restricted FDS polling policy. Using this mode, FDS will poll only those services who either live in an explicitly whitelisted namespace, or themselves are are explicitly whitelisted. *`WHITELIST` is the default mode for FDS*.
* `DISABLED`: FDS will not run. **Upstream Discovery Service** (UDS) will still run as normal.

Setting the `fdsMode` can be done either via the Helm Chart, or by directly modifying the `default` `gloo.solo.io/v1.Settings` custom resource in Gloo Edge's installation namespace (`gloo-system`).

(Enterprise Only) Automated schema generation for GraphQL is enabled by default. This can be disabled by modifying the `gloo.solo.io/v1.Settings` custom resource as seen [below](#setting-fdsmode-by-editing-the-gloosoloiov1settings-custom-resource)

### Setting `fdsMode` via the Helm chart

Add the following to your Helm overrides file:
```yaml
settings:
  create: true

discovery:
  # set to either WHITELIST, BLACKLIST, or DISABLED
  # WHITELIST is the default value
  fdsMode: BLACKLIST
```

Or add the following CLI flags to `helm install|template` commands:

```bash
helm install|template ... --set settings.create=true --set discovery.fdsMode=BLACKLIST
```

### Setting `fdsMode` by editing the `gloo.solo.io/v1.Settings` custom resource:

```bash
kubectl edit -n gloo-system settings.gloo.solo.io
```
{{< highlight yaml "hl_lines=20-28" >}}
# Please edit the object below. Lines beginning with a '#' will be ignored,
# and an empty file will abort the edit. If an error occurs while saving this file will be
# reopened with the relevant failures.
#
apiVersion: gloo.solo.io/v1
kind: Settings
metadata:
  labels:
    app: gloo
    gloo: settings
  name: default
  namespace: gloo-system
spec:
  bindAddr: 0.0.0.0:9977
  discoveryNamespace: gloo-system
  kubernetesArtifactSource: {}
  kubernetesConfigSource: {}
  kubernetesSecretSource: {}
  refreshRate: 60s
  # add the following lines
  discovery:
    # set to either WHITELIST, BLACKLIST, or DISABLED
    # WHITELIST is the default value
    fdsMode: WHITELIST
    fdsOptions:
      # set to false to disable automated GraphQL schema generation as part of FDS.
      # true is the default value (enabled)
      graphqlEnabled: true
{{< /highlight >}}

---

## Blacklisting Namespaces & Upstreams

When running in `BLACKLIST` mode, blacklist upstreams by adding the following label:

`discovery.solo.io/function_discovery=disabled`.

E.g. with

```bash
kubectl label namespace default discovery.solo.io/function_discovery=disabled
kubectl label upstream -n myapp myupstream discovery.solo.io/function_discovery=disabled

# if the Upstream was discovered and is managed by UDS (Upstream Discovery Service)
# then you can add the label to the service and it will propagate to the Upstream
kubectl label service -n myapp myservice discovery.solo.io/function_discovery=disabled
```

This label can be applied to namespaces and upstreams.

To enable FDS for specific upstreams in a blacklisted namespace:

`discovery.solo.io/function_discovery=enabled`

---

## Whitelisting Namespaces & Upstreams

When running in `WHITELIST` mode, whitelist `Upstreams` by adding the following label:

`discovery.solo.io/function_discovery=enabled`.

E.g. with

```bash
kubectl label namespace default discovery.solo.io/function_discovery=enabled
kubectl label upstream -n myapp myupstream discovery.solo.io/function_discovery=enabled

# if the Upstream was discovered and is managed by UDS (Upstream Discovery Service)
# then you can add the label to the service and it will propagate to the Upstream
kubectl label service -n myapp myservice discovery.solo.io/function_discovery=enabled
```

This label can be applied to namespaces and upstreams.

To disable FDS for specific services/upstreams in a whitelisted namespace:

`discovery.solo.io/function_discovery=disabled`
