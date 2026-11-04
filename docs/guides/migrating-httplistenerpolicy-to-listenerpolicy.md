# Migrating from `HTTPListenerPolicy` to `ListenerPolicy`

The `HTTPListenerPolicy` CRD has been removed. Everything it configured is available on
`ListenerPolicy` under `spec.default.httpSettings`.

This guide shows how to port your manifests. The migration is mechanical: **no field was renamed,
retyped, or dropped** — the entire `HTTPListenerPolicy` spec body is the same `httpSettings` object
that `ListenerPolicy` already accepts. In most cases you change `kind` and re-indent.

> [!IMPORTANT]
> Migrate before you upgrade. The `HTTPListenerPolicy` CRD is removed from the `kgateway-crds`
> chart, so upgrading deletes any remaining `HTTPListenerPolicy` objects in the cluster along with
> the configuration they were applying. See [Upgrade order](#upgrade-order).

## The change at a glance

Before:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: HTTPListenerPolicy
metadata:
  name: my-policy
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: my-gateway
  useRemoteAddress: true
  xffNumTrustedHops: 2
  streamIdleTimeout: 30s
```

After:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: ListenerPolicy
metadata:
  name: my-policy
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: my-gateway
  default:
    httpSettings:
      useRemoteAddress: true
      xffNumTrustedHops: 2
      streamIdleTimeout: 30s
```

The recipe, in three steps:

1. `kind: HTTPListenerPolicy` → `kind: ListenerPolicy`. `apiVersion` is unchanged
   (`gateway.kgateway.dev/v1alpha1`).
2. Leave `targetRefs` and `targetSelectors` where they are.
3. Move every other top-level `spec` field under `spec.default.httpSettings`, indenting by four
   spaces.

## Field mapping

This is the complete set of fields that move from `spec.<field>` to
`spec.default.httpSettings.<field>`. Names, types and semantics are identical; the table is laid out
in three columns for brevity and has no other meaning.

| | | |
| --- | --- | --- |
| `accessLog` | `acceptHttp10` | `defaultHostForHttp10` |
| `tracing` | `preserveHttp1HeaderCase` | `earlyRequestHeaderModifier` |
| `localReplies` | `http2ProtocolOptions` | `forwardClientCertDetails` |
| `upgradeConfig` | `healthCheck` | `uuidRequestIdConfig` |
| `useRemoteAddress` | `serverName` | `stripHostPortMode` |
| `preserveExternalRequestId` | `serverHeaderTransformation` | `maxRequestHeadersKb` |
| `generateRequestId` | `streamIdleTimeout` | `maxRequestsPerConnection` |
| `xffNumTrustedHops` | `idleTimeout` | `maxHeadersCount` |
| `xffTrustedCIDRs` | `skipXFFAppend` | `proxy100Continue` |

`targetRefs` and `targetSelectors` stay at `spec` level.

## Worked examples

### Access logging

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: ListenerPolicy
metadata:
  name: access-logs
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: my-gateway
  default:
    httpSettings:
      accessLog:
      - fileSink:
          path: /dev/stdout
          jsonFormat:
            start_time: "%START_TIME%"
            method: "%REQ(X-ENVOY-ORIGINAL-METHOD?:METHOD)%"
            path: "%REQ(X-ENVOY-ORIGINAL-PATH?:PATH)%"
            response_code: "%RESPONSE_CODE%"
```

### Tracing

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: ListenerPolicy
metadata:
  name: tracing-policy
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: my-gateway
  default:
    httpSettings:
      tracing:
        provider:
          openTelemetry:
            serviceName: "my-gateway"
            grpcService:
              backendRef:
                name: otel-collector
                namespace: default
                port: 4317
        spawnUpstreamSpan: true
        attributes:
        - name: custom
          literal:
            value: literal
```

### Label-based attachment

`targetSelectors` works the same way:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: ListenerPolicy
metadata:
  name: access-logs
  namespace: infra
spec:
  targetSelectors:
  - group: gateway.networking.k8s.io
    kind: Gateway
    matchLabels:
      gateway: example
  default:
    httpSettings:
      accessLog:
      - grpcService:
          logName: "test-accesslog-service"
          backendRef:
            name: log-test
            port: 50051
```

## What you gain

`ListenerPolicy` is a superset of `HTTPListenerPolicy`, so migrating opens up configuration that was
previously unavailable.

### Listener-level settings alongside HTTP settings

`spec.default` also carries connection- and listener-level configuration — `proxyProtocol`,
`tcpKeepalive`, `perConnectionBufferLimitBytes`, `transportSocketConnectTimeout` and
`clientCertificateValidation` — so one object can now express what used to need two:

```yaml
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: my-gateway
  default:
    perConnectionBufferLimitBytes: 1048576
    transportSocketConnectTimeout: 10s
    tcpKeepalive:
      keepAliveProbes: 3
      keepAliveTime: 30s
      keepAliveInterval: 5s
    httpSettings:
      useRemoteAddress: true
```

### Per-port configuration

`spec.perPort` scopes configuration to listeners on a given port:

```yaml
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: my-gateway
  default:
    httpSettings:
      useRemoteAddress: true
  perPort:
  - port: 8443
    listener:
      httpSettings:
        useRemoteAddress: false
        xffNumTrustedHops: 1
```

> [!WARNING]
> A `perPort` entry **replaces** `default` outright for listeners on that port — it does not merge
> field-by-field. Fields you leave unset in a `perPort` entry are unset, not inherited from
> `default`. Repeat any `default` values you still want on that port.

### Attaching to a single listener

`targetRefs` and `targetSelectors` accept an optional `sectionName`, which `HTTPListenerPolicy` did
not support:

```yaml
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: my-gateway
    sectionName: https
```

## Behaviour differences to check

Config translation is unchanged — the same input produces the same Envoy `HttpConnectionManager`.
Two observable things do change.

### Policy status moves to the new object

Status is reported on the `ListenerPolicy` object, in the same `status.ancestors` shape. Anything
that polls `HTTPListenerPolicy` status — scripts, CI gates, GitOps health checks — needs to point
at the new kind:

```bash
kubectl get listenerpolicy my-policy -o jsonpath='{.status.ancestors[*].conditions[?(@.type=="Accepted")].status}'
```

### Merge-origin metadata is now fully qualified

kgateway records which policy contributed each merged field in the
`merge.<Kind>.gateway.kgateway.dev` filter metadata on the listener and route config. The key is the
field's path in the spec, so it now reflects the nesting:

```diff
-      merge.HTTPListenerPolicy.gateway.kgateway.dev:
-        xffNumTrustedHops:
-        - gateway.kgateway.dev/HTTPListenerPolicy/default/policy-1
+      merge.ListenerPolicy.gateway.kgateway.dev:
+        default.httpSettings.xffNumTrustedHops:
+        - gateway.kgateway.dev/ListenerPolicy/default/policy-1
```

This is debug/observability metadata and does not affect routing. It matters only if you assert on
`config_dump` output or scrape these keys.

Policies that were already `ListenerPolicy` are unaffected — they always emitted the qualified form.
Only settings you migrate off `HTTPListenerPolicy` change shape here.

### Merging between policies is unchanged

Multiple `ListenerPolicy` objects attached to the same Gateway merge exactly as multiple
`HTTPListenerPolicy` objects did, with the same precedence rules, so splitting one policy into
several — or combining several into one — is safe. See
[policy merging](../../devel/policy_merging/overview.md) for the details.

Note that merging only ever applied *within* a kind. An `HTTPListenerPolicy` and a `ListenerPolicy`
on the same Gateway were applied as two independent translation passes, not merged, so for any HTTP
setting configured in both, which one won was not well defined. This matters only during migration —
see [Upgrade order](#upgrade-order).

## Upgrade order

The CRD is deleted by the chart upgrade, and Kubernetes garbage-collects the custom resources with
it. Migrate first:

1. Find what you have:

   ```bash
   kubectl get httplistenerpolicies.gateway.kgateway.dev -A
   ```

2. Back them up:

   ```bash
   kubectl get httplistenerpolicies.gateway.kgateway.dev -A -o yaml > httplistenerpolicies-backup.yaml
   ```

3. Convert each one, then **replace** the old object with the new one: apply the `ListenerPolicy`
   and delete the `HTTPListenerPolicy` it came from.

   > [!WARNING]
   > Do not leave an `HTTPListenerPolicy` and a `ListenerPolicy` configuring the same setting on the
   > same Gateway. The two kinds were applied as separate translation passes rather than merged, so
   > for a field set in both, which value reaches Envoy is not well defined. If you want to stage
   > the change, verify on a non-production Gateway rather than running both against the same one.

4. Confirm the new policies are attached:

   ```bash
   kubectl get listenerpolicy -A
   ```

   Look for `Accepted=True` and `Attached=True`.

5. Confirm nothing is left behind:

   ```bash
   kubectl get httplistenerpolicies.gateway.kgateway.dev -A
   ```

6. Upgrade kgateway.

If you upgrade before migrating, restore from the backup taken in step 2, converting each object as
you go — the exported YAML still contains the full spec.

## Verifying the result

Confirm the generated Envoy config is what you expect:

```bash
kubectl port-forward deployment/<gateway-deployment> 19000:19000
curl -s localhost:19000/config_dump | jq '.configs[] | select(.["@type"] | contains("ListenersConfigDump"))'
```

The `http_connection_manager` fields should match what the `HTTPListenerPolicy` produced before the
migration.
