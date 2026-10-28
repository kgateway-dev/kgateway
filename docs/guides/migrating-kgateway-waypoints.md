# Migrating kgateway waypoints to Istio waypoints

Support for running kgateway as an Istio ambient waypoint has been removed. That covers the
`kgateway-waypoint` GatewayClass, kgateway's translation of Istio `AuthorizationPolicy`, and the
`KGW_WAYPOINT_LOCAL_BINDING` setting. Use Istio's own waypoint, the `istio-waypoint` GatewayClass,
instead.

Using kgateway as the **ingress** gateway in front of Istio waypoints is still supported. See
[Sending ingress traffic through Istio waypoints](#sending-ingress-traffic-through-istio-waypoints).

> [!IMPORTANT]
> Migrate before you upgrade. After the upgrade, kgateway stops managing waypoint Gateways, and a
> waypoint pod that restarts gets no configuration. See [What happens on upgrade](#what-happens-on-upgrade).

## What happens on upgrade

kgateway no longer reconciles a `kgateway-waypoint` GatewayClass, or any GatewayClass carrying the
`ambient.istio.io/waypoint-inbound-binding` annotation that kgateway set on it, or the Gateways that
use it. It does not migrate them, and it does not delete them:

- The GatewayClass and its Gateways stay in the cluster. Their status is no longer updated, so a
  stale `Accepted=True` or `Programmed=True` does not mean they work.
- Their Deployments and Services stay as they were, still running the previous kgateway proxy image.
- Waypoint proxies that are already running keep serving the configuration they last received. A
  waypoint pod that restarts, for example after an eviction or a node drain, gets no configuration,
  and traffic through it fails.

## Migrating a waypoint

Repeat these steps for each kgateway waypoint. The ztunnel switches a workload to a different
waypoint as soon as its `istio.io/use-waypoint` label changes, so you can move one namespace or
Service at a time.

### 1. Create an Istio waypoint

Create the Istio waypoint next to the old one, under a new name. For example, for this kgateway
waypoint:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: httpbin-waypoint
  namespace: httpbin
spec:
  gatewayClassName: kgateway-waypoint
  listeners:
  - name: mesh
    port: 15088
    protocol: istio.io/PROXY
```

create:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: httpbin-istio-waypoint
  namespace: httpbin
spec:
  gatewayClassName: istio-waypoint
  listeners:
  - name: mesh
    port: 15008
    protocol: HBONE
```

Wait for it to report `Programmed=True`. See the Istio documentation on configuring waypoint
proxies for the options it supports, such as `istio.io/waypoint-for`.

### 2. Move routes and policies to the new waypoint

Anything that names the old waypoint Gateway must name the new one instead:

- **HTTPRoutes and GRPCRoutes** that use the old Gateway as a `parentRef`. Routes whose `parentRef`
  is a Service or ServiceEntry need no change: they apply to whichever waypoint that Service uses.
- **AuthorizationPolicies** whose `targetRefs` name the old Gateway. Policies that targeted the
  `kgateway-waypoint` GatewayClass must target `istio-waypoint` instead, in the mesh root namespace.
  Check your Istio version's documentation for which `targetRefs` kinds its waypoint supports.
- **kgateway policies**, such as TrafficPolicy, that targeted the old Gateway or its routes. Istio
  waypoints do not apply kgateway policies. Express that configuration with Istio APIs, or drop it.

kgateway translated AuthorizationPolicy with its own implementation. Istio's waypoint enforces the
same API, but review policies that use `CUSTOM` actions or uncommon match fields, and test them in
the next step rather than assuming identical behaviour.

### 3. Switch workloads to the new waypoint

Change the `istio.io/use-waypoint` label on each namespace, Service or ServiceEntry from the old
waypoint's name to the new one:

```bash
kubectl label namespace httpbin istio.io/use-waypoint=httpbin-istio-waypoint --overwrite
```

### 4. Verify, then delete the old waypoint

Before deleting anything, check that:

- traffic to the moved Services succeeds and goes through the new waypoint, for example by checking
  the waypoint's access logs or `istioctl ztunnel-config workloads`;
- AuthorizationPolicies allow and deny the requests they did before;
- nothing still references the old Gateway.

Then delete the old Gateway. Once no Gateway uses it, delete the `kgateway-waypoint` GatewayClass.
kgateway no longer recreates it.

## Sending ingress traffic through Istio waypoints

kgateway can still send ingress traffic for a Service through that Service's Istio waypoint, so the
waypoint's policies and telemetry apply to it. Turn it on with the Helm value `waypoint.enabled`, or
without Helm by setting `KGW_INGRESS_USE_WAYPOINTS=true`. The older `KGW_ENABLE_WAYPOINT=true` still
works when `KGW_INGRESS_USE_WAYPOINTS` is unset, but it is deprecated.

With it on, kgateway sends a backend's traffic to the Service's cluster IP instead of to its pods,
and ztunnel passes it through the waypoint. That only happens when all of these hold:

- **The ingress gateway's pods are in the ambient mesh**, so ztunnel captures their traffic: put the
  gateway's namespace or pods in ambient mode with `istio.io/dataplane-mode=ambient`. Gateways
  outside the mesh keep sending straight to the pods.
- **The backend opts in** with the label `istio.io/ingress-use-waypoint=true` on the Service or
  ServiceEntry, or on its namespace.
- **The backend has an Istio waypoint**, attached with `istio.io/use-waypoint`.
- **The backend has a cluster IP.** A headless Service (`clusterIP: None`) has none, so its cluster
  has no endpoints and requests fail with `503` (response flag `UH`). Leave the label off headless
  Services.
- **For ServiceEntries, Istio integration is enabled** with `KGW_ENABLE_ISTIO_INTEGRATION=true`.
  Without it, kgateway does not use ServiceEntries as backends at all.
