# Zone-Aware Routing

This guide explains how to enable zone-aware routing in kgateway and, most importantly, how to configure the proxy locality that Envoy needs for the feature to work.

## Overview

kgateway exposes zone-aware routing through `BackendConfigPolicy.spec.loadBalancer.zoneAware.preferLocal`.

Envoy can only apply zone-aware routing when the proxy knows its own locality. In kgateway, the proxy locality is read from these environment variables on the Envoy pod:

- `KGATEWAY_NODE_REGION`
- `KGATEWAY_NODE_ZONE`
- `KGATEWAY_NODE_SUBZONE`

If these variables are not set, Envoy bootstrap does not get `node.locality`, and zone-aware routing will not take effect.

## Important default behavior

By default, the Envoy deployment template sets `NODE_NAME`, but it does not automatically populate `KGATEWAY_NODE_REGION`, `KGATEWAY_NODE_ZONE`, or `KGATEWAY_NODE_SUBZONE` from the node labels.

That means enabling `zoneAware` in a `BackendConfigPolicy` is not sufficient by itself. You must also configure the proxy locality on the Envoy pod.

## Safe configuration pattern

The safest configuration pattern with the current deployment model is:

1. Pin the gateway proxy to a known zone.
2. Set matching `KGATEWAY_NODE_*` environment variables on that proxy.
3. Enable zone-aware routing on the target backend.

This avoids advertising a proxy locality that does not match the node where the proxy is actually running.

## Configure proxy locality with GatewayParameters

The example below pins the proxy to `us-east-1a` and sets matching locality environment variables.

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: GatewayParameters
metadata:
  name: zone-aware-gateway-params
  namespace: kgateway-system
spec:
  kube:
    podTemplate:
      nodeSelector:
        topology.kubernetes.io/zone: us-east-1a
    envoyContainer:
      env:
        - name: KGATEWAY_NODE_REGION
          value: us-east-1
        - name: KGATEWAY_NODE_ZONE
          value: us-east-1a
        - name: KGATEWAY_NODE_SUBZONE
          value: rack-a
```

Attach that `GatewayParameters` resource to the gateway you want to use for zone-aware routing.

## Enable zone-aware routing on the backend

After the proxy locality is configured, enable zone-aware routing with `BackendConfigPolicy`.

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: BackendConfigPolicy
metadata:
  name: zone-aware-backend
  namespace: default
spec:
  targetRefs:
    - group: ""
      kind: Service
      name: example-backend
  loadBalancer:
    roundRobin: {}
    zoneAware:
      preferLocal:
        minEndpointsThreshold: 6
        routingEnabled: 100
```

If you want stricter same-zone behavior while enough local endpoints are available, add `force`:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: BackendConfigPolicy
metadata:
  name: zone-aware-backend
  namespace: default
spec:
  targetRefs:
    - group: ""
      kind: Service
      name: example-backend
  loadBalancer:
    roundRobin: {}
    zoneAware:
      preferLocal:
        minEndpointsThreshold: 6
        routingEnabled: 100
        force:
          minEndpointsInZoneThreshold: 1
```

## Operational notes

- The `KGATEWAY_NODE_*` values must match the actual node locality of the proxy.
- If you hardcode locality values, pin the proxy to the matching zone.
- If you source the values from the Kubernetes Downward API, put the locality values on the proxy pod as labels or annotations first. Envoy cannot read Kubernetes node labels directly.
- If the proxy moves to another zone without the env vars changing, Envoy will advertise the wrong locality.
- Zone-aware routing also depends on upstream endpoint locality metadata being present.

## Current limitation

kgateway does not currently auto-populate `KGATEWAY_NODE_REGION`, `KGATEWAY_NODE_ZONE`, or `KGATEWAY_NODE_SUBZONE` from Kubernetes node labels in the default deployment template.

Until that is automated, operators must configure those values explicitly on the Envoy pod when using zone-aware routing.
