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

Mirror the node's zone labels onto the proxy pods, then read them with the Downward API:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: GatewayParameters
metadata:
  name: zone-aware-gateway-params
  namespace: kgateway-system
spec:
  kube:
    envoyContainer:
      env:
        - name: KGATEWAY_NODE_REGION
          valueFrom:
            fieldRef:
              fieldPath: metadata.labels['topology.kubernetes.io/region']
        - name: KGATEWAY_NODE_ZONE
          valueFrom:
            fieldRef:
              fieldPath: metadata.labels['topology.kubernetes.io/zone']
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
- If you use the Downward API for env vars, ensure the locality labels/annotations are present at pod creation time. Environment variables are not updated if labels are added later, so a pod that starts without them will advertise no locality until it is restarted.- If the proxy moves to another zone without the env vars changing, Envoy will advertise the wrong locality.
- Zone-aware routing also depends on upstream endpoint locality metadata being present.

## Current limitation

kgateway does not currently auto-populate `KGATEWAY_NODE_REGION`, `KGATEWAY_NODE_ZONE`, or `KGATEWAY_NODE_SUBZONE` from Kubernetes node labels in the default deployment template, and kgateway does not copy node zone labels onto proxy pods.

Until that is automated, operators must configure those values explicitly on the Envoy pod when using zone-aware routing.

## Run the zone-aware routing e2e test

The zone-aware routing end-to-end test exercises the full feature on a local kind cluster:

1. even distribution without a policy
2. `preferLocal` keeping traffic in the local zone
3. cross-zone spillover when local capacity is insufficient
4. `force` keeping traffic local regardless of capacity

The test needs to be manually run locally as no current test runners are configured with multiple nodes, so running it in CI requires modification to the CI which is deferred to later.

To run the test locally, create the test cluster first.
The setup script creates a kind cluster with three worker nodes labeled as zones `us-east-1a`, `us-east-1b`, and `us-east-1c`, then builds the kgateway images and deploys kgateway via `make run`.
If a cluster with the same name already exists, it is reused instead of recreated:

```sh
./hack/kind/setup-zone-aware-routing.sh
```

Then run the test:

```sh
go test -tags=e2e -vet=off -timeout=20m ./test/e2e/tests -run '^TestZoneAwareRouting$' -count=1
```

The cluster name defaults to `kgw-zone-aware` on both sides. To use a different cluster, pass `CLUSTER_NAME=<name>` to the setup script and `ZONE_AWARE_CLUSTER_NAME=<name>` to the test.
