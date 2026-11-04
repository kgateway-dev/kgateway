# Zone-Aware Routing

This guide explains how to enable zone-aware routing in kgateway.

## Overview

kgateway exposes zone-aware routing through `BackendConfigPolicy.spec.loadBalancer.zoneAware.preferLocal`.
When enabled, the proxy prefers sending traffic to backend endpoints in its own availability
zone, reducing cross-zone data transfer cost and latency while maintaining overall balance
across zones.

## Prerequisites

- Kubernetes 1.35 or newer. For older clusters, see [Manual overrides and clusters < 1.35](#manual-overrides-and-clusters--135).
- Nodes have `topology.kubernetes.io/region` and `topology.kubernetes.io/zone` labels.
  All major cloud providers set these automatically when nodes register.

## Enable zone-aware routing

Enable zone-aware routing with a `BackendConfigPolicy` targeting the backend Service:

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
      preferLocal: {}
```

For strict same-zone behavior that holds even when the local zone has less than its
proportional share of endpoints, use `force`:

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
        force: {}
```

With `force`, Envoy routes only to same-zone endpoints as long as the local zone has at least
`force.minEndpointsInZoneThreshold` endpoints (default 1); below that it falls back to standard
zone-aware behavior.

### Notes

- The proxy reads its locality once at startup. This is safe because a pod's zone can never
  change while it runs, but if nodes are labeled *after* gateway pods started, restart the pods
  to pick up the labels.
- `preferLocal.minEndpointsThreshold` defaults to **6**. If the backend has fewer total
  endpoints than this threshold, zone-aware routing is disabled entirely and traffic spreads
  across all zones (`lb_zone_cluster_too_small`). This applies even when `force` is set. For
  small backends, set the threshold explicitly.
- `topology.istio.io/subzone` is not automatically copied to pod labels, as it is not a
  Kubernetes-standard label. To include subzone in zone-aware routing, see [Manual overrides and clusters < 1.35](#manual-overrides-and-clusters--135).

## Verify and debug

Check that the proxy pod received the topology labels at scheduling:

```sh
kubectl get pod <gateway-pod> -n <namespace> -o jsonpath='{.metadata.labels.topology\.kubernetes\.io/zone}'
```

Check the locality Envoy actually loaded:

```sh
kubectl exec <gateway-pod> -n <namespace> -- wget -qO- http://127.0.0.1:19000/config_dump \
  | grep -A4 '"locality"'
```

Check whether zone-aware routing is engaging, using the cluster's zone routing stats:

```sh
kubectl exec <gateway-pod> -n <namespace> -- wget -qO- http://127.0.0.1:19000/stats \
  | grep lb_zone
```

- `lb_zone_routing_all_directly` / `lb_zone_routing_sampled` increasing: zone-aware is active.
- `lb_zone_cluster_too_small` increasing: the backend has fewer endpoints than
  `preferLocal.minEndpointsThreshold`; lower the threshold or scale the backend.
- `lb_local_cluster_not_ok` increasing briefly right after proxy startup is normal (the local
  cluster's endpoints have not been delivered yet); if it keeps increasing, the proxy has no
  usable locality.

## Manual overrides and clusters < 1.35

The proxy locality can be set explicitly with environment variables on the Envoy container,
configured through `GatewayParameters`:

- `KGATEWAY_NODE_REGION`
- `KGATEWAY_NODE_ZONE`
- `KGATEWAY_NODE_SUBZONE`

Each variable overrides the corresponding pod topology label. There are two ways to source the
values.

### 1. Hardcoded values

Works on any cluster, but proxy pods need to be pinned to the matching zone.

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: GatewayParameters
metadata:
  name: zone-pinned-gateway-params
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
          value: rack-1
```

### 2. Manually adding labels

Manually add labels `topology.kubernetes.io/region`, `topology.kubernetes.io/zone`,
and `topology.istio.io/subzone` to the proxy pods.

Note: on Kubernetes 1.35 or newer, manually set `topology.kubernetes.io/region` and
`topology.kubernetes.io/zone` pod labels are overwritten with the node's values at
scheduling; `topology.istio.io/subzone` is untouched.

## Run the zone-aware routing e2e test

The zone-aware routing end-to-end test exercises the full feature on a local kind cluster:

1. Even distribution without a policy
2. `preferLocal` keeping traffic in the local zone
3. Cross-zone spillover when local capacity is insufficient
4. `force` keeping traffic local regardless of capacity

The test needs to be manually run locally as no current test runners are configured with multiple
nodes, so running it in CI requires modification to the CI which is deferred to later.

To run the test locally, create the test cluster first. The setup script creates a kind cluster
with three worker nodes labeled as zones `us-east-1a`, `us-east-1b`, and `us-east-1c`, then
builds the kgateway images and deploys kgateway via `make run`. If a cluster with the same name
already exists, it is reused instead of recreated:

```sh
./hack/kind/setup-zone-aware-routing.sh
```

Then run the test:

```sh
go test -tags=e2e -vet=off -timeout=20m ./test/e2e/tests -run '^TestZoneAwareRouting$' -count=1
```

The cluster name defaults to `kgw-zone-aware` on both sides. To use a different cluster, pass
`CLUSTER_NAME=<name>` to the setup script and `ZONE_AWARE_CLUSTER_NAME=<name>` to the test.
