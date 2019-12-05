---
title: Configuring Function Discovery
weight: 10
description: Using automatic function discovery (ie, discovering and understanding Swagger/OAS docs or gRPC reflection)
---

## Motivation
Gloo's **Function Discovery Service** (FDS) attempts to 
poll service endpoints for:

* A path serving a [Swagger Document](https://swagger.io/specification/).
* gRPC Services with [gRPC Reflection](https://github.com/grpc/grpc/blob/master/doc/server-reflection.md) enabled.

This means that the Gloo `discovery` pod/binary will make 
HTTP requests to all services known to Gloo. 

This behavior causes increased network traffic and may 
be undesirable if it causes unexpected behavior or logs
to appear in the services Gloo is attempting to poll.

For this reason, we may want to restrict the manner in 
which FDS polls services.

Gloo allows whitelisting/blacklisting services, either by namespace or on the individual service level.

We can use these configuration settings to restrict 
FDS to discover only the namespaces or individual 
services we choose.

## Configuring the `fdsMode` Setting

FDS can run in one of 3 modes:

* `BLACKLIST`: The most liberal FDS polling policy. Using this mode, FDS will poll any service unless its namespace or the service itself is explicitly blacklisted.
* `WHITELIST`: A more restricted FDS polling policy. Using this mode, FDS will poll only those services who either live in an explicitly whitelisted namespace, or themselves are are explicitly whitelisted. *`WHITELIST` is the default mode for FDS*.
* `DISABLED`: FDS will not run. **Upstream Discovery Service** (UDS) will still run as normal.

Setting the `fdsMode` can be done either via the Helm Chart, or by directly modifying the `default` `gloo.solo.io/v1.Settings` custom resource in Gloo's installation namespace (`gloo-system`).

### Setting `fdsMode` via the Helm chart:

Add the following to your Helm overrides file: 
```yaml
settings:
  create: true

discovery:
  # set to either WHITELIST, BLACKLIST, or DISABLED
  # BLACKLIST is the default value
  fdsMode: WHITELIST 
```

Or add the following CLI flags to `helm install|template` commands:

```bash
helm install|template ... --set settings.create=true --set discovery.fdsMode=WHITELIST
```

### Settings `fdsMode` by editing the `gloo.solo.io/v1.Settings` custom resource:

```bash
kubectl edit -n gloo-system settings.gloo.solo.io
```
{{< highlight yaml "hl_lines=20-24" >}}
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
    # BLACKLIST is the default value
    fdsMode: WHITELIST
{{< /highlight >}}

## Blacklisting Namespaces & Services

When running in `BLACKLIST` mode, blacklist services by adding the following label:

`discovery.solo.io/function_discovery=disabled`.

E.g. with

```bash
kubectl label namespace default discovery.solo.io/function_discovery=disabled
kubectl label service -n myapp myservice discovery.solo.io/function_discovery=disabled
```

This label can be applied to namespaces, services, and upstreams.

To enable FDS for specific services/upstreams in a blacklisted namespace:

`discovery.solo.io/function_discovery=enabled`

## Whitelisting Namespaces & Services

When running in `WHITELIST` mode, whitelist services by adding the following label:

`discovery.solo.io/function_discovery=enabled`.

E.g. with

```bash
kubectl label namespace default discovery.solo.io/function_discovery=enabled
kubectl label service -n myapp myservice discovery.solo.io/function_discovery=enabled
```

This label can be applied to namespaces, services, and upstreams.

To disble FDS for specific services/upstreams in a whitelisted namespace:

`discovery.solo.io/function_discovery=disabled`
