# TLS Configuration Ordering in kgateway

This document describes the order in which TLS-related plugins are applied when configuring backend connections in kgateway, and how conflicts between different TLS configurations are resolved.

## TLS Configuration Behavior

### BackendTLSPolicy Plugin

- **Purpose**: Implements the Gateway API `BackendTLSPolicy` resource
- **TLS Configuration**: Sets the `TransportSocket` on the Envoy cluster for backend connections
- **Key Behavior**: 
  - Directly sets `out.TransportSocket` on the cluster
  - **Server-side TLS validation only** (validates server certificates)
  - Uses ConfigMaps for CA certificate references
  - Implements SNI (Server Name Indication) support
  - **Does NOT support mTLS** (client certificate authentication)

### BackendConfigPolicy Plugin

- **Purpose**: Implements the kgateway-specific `BackendConfigPolicy` resource
- **TLS Configuration**: Also sets the `TransportSocket` on the Envoy cluster
- **Key Behavior**:
  - **Overwrites** any existing `TransportSocket` configuration
  - Supports comprehensive TLS features including:
    - Secret references and file-based certificates
    - Subject Alternative Name (SAN) verification
    - TLS parameters and ALPN protocols
    - **Simple TLS vs mTLS configuration** via `SimpleTLS` flag
    - TLS renegotiation control
    - **Client certificate authentication** (mTLS)

## Istio Plugin Integration

The **Istio plugin** ([applied in the plugin registry](https://github.com/kgateway-dev/kgateway/blob/main/internal/kgateway/extensions2/registry/registry.go)) can also configure TLS through:

- **Istio mTLS**: Uses `ISTIO_MUTUAL` mode for automatic mutual TLS
- **Transport Socket Matches**: Creates two transport socket matches - one for Istio mTLS and one for cleartext
- **SDS Integration**: Uses Istio's Secret Discovery Service for the mTLS certificate management

### Istio Auto-mTLS Behavior

By default, when Istio is enabled:
- **In-mesh services**: Traffic uses Istio's automatic mTLS
- **External services**: Traffic uses cleartext (no TLS)

However, this behavior can be **disabled per-backend** using the `kgateway.dev/disable-istio-auto-mtls: "true"` annotation on any backend object:
- **Kubernetes Services** (for in-mesh backends)
- **Backend resources** (for external backends)
- **ServiceEntry resources** (for external services)

## Conflict Resolution

### TLS Transport Socket Conflicts

When both `BackendTLSPolicy` and `BackendConfigPolicy` are applied to the same backend:

1. **BackendTLSPolicy** is applied first and sets the initial `TransportSocket`
2. **BackendConfigPolicy** is applied second and **overwrites** the `TransportSocket` if it has TLS configuration

This means that **BackendConfigPolicy takes precedence** over BackendTLSPolicy for TLS configuration.

**Note**: This ordering is from lower→higher priority (BackendConfigPolicy overwrites BackendTLSPolicy), which is the opposite of built-in vs TrafficPolicy where built-in has higher priority and TrafficPolicy must check if fields are set before overwriting.

### TLS Configuration Precedence with Istio

The Istio plugin's TLS configuration is applied before both BackendTLSPolicy and BackendConfigPolicy. The precedence depends on whether auto-mTLS is disabled:

1. **When auto-mTLS is enabled** (default): Istio's TransportSocketMatches take precedence
2. **When auto-mTLS is disabled** (via annotation): BackendConfigPolicy and BackendTLSPolicy can take effect
3. **When both are present**: BackendConfigPolicy > BackendTLSPolicy > Istio (if disabled)

## Policy Application Ordering

As of [PR #11297](https://github.com/kgateway-dev/kgateway/pull/11297), the policy application system uses `ApplyOrderedGroupKinds()` which ensures:

1. **Built-in policies** (VirtualBuiltInGK) are applied first (highest priority)
2. **Other policies** are applied in their registration order
3. **Policy merging** is performed by GroupKind before application

This means that within each plugin, multiple policies of the same type are merged before being applied, ensuring consistent behavior.

## Disabling Istio Auto-mTLS

To allow BackendConfigPolicy or BackendTLSPolicy to take effect when Istio is enabled, add the `kgateway.dev/disable-istio-auto-mtls: "true"` annotation to your backend resource:

```yaml
apiVersion: kgateway.io/v1alpha1
kind: Backend
metadata:
  name: external-backend
  annotations:
    kgateway.dev/disable-istio-auto-mtls: "true"
spec:
  # ... backend spec
```

This annotation:
- **Disables** Istio's automatic mTLS for this specific backend
- **Allows** BackendConfigPolicy and BackendTLSPolicy to configure TLS instead
- **Applies** only to the annotated resource, not globally

## Best Practices

1. **Use BackendConfigPolicy for advanced TLS features** - It supports more comprehensive TLS configuration options
2. **Avoid mixing BackendTLSPolicy and BackendConfigPolicy** - BackendConfigPolicy will override BackendTLSPolicy
3. **Disable Istio auto-mTLS when needed** - Use the `kgateway.dev/disable-istio-auto-mtls` annotation to allow custom TLS policies
4. **Consider Istio integration** - If using Istio mTLS, ensure your policies don't conflict with Istio's automatic TLS configuration
5. **Test TLS configuration thoroughly** - The last-applied policy wins, so verify the final configuration meets your security requirements

## TLS Configuration Behavior

### Default Behavior (Istio Auto-mTLS Enabled)

When Istio is enabled and the `kgateway.dev/disable-istio-auto-mtls` annotation is either **not present** or set to anything other than `"true"`:

- **In-mesh backends** (Kubernetes Services): Istio automatically applies mTLS using TransportSocketMatches
- **External backends** (Backend resources): Traffic uses cleartext (no TLS)
- **BackendConfigPolicy and BackendTLSPolicy**: Are ignored for TLS configuration

### Custom TLS Behavior (Istio Auto-mTLS Disabled)

When the `kgateway.dev/disable-istio-auto-mtls: "true"` annotation is **present** on the backend resource:

- **Istio auto-mTLS**: Is disabled for this specific backend
- **BackendConfigPolicy**: Can configure TLS and will take precedence over BackendTLSPolicy
- **BackendTLSPolicy**: Can configure TLS but will be overridden by BackendConfigPolicy if both are present
- **Plugin application order**: BackendTLSPolicy → BackendConfigPolicy (last wins for TLS)

### Key Points

1. **Per-backend control**: The annotation works on both Kubernetes Services and Backend resources
2. **Selective disabling**: You can disable auto-mTLS for specific backends while keeping it enabled for others
3. **Policy precedence**: When auto-mTLS is disabled, BackendConfigPolicy > BackendTLSPolicy > Istio
4. **TransportSocket vs TransportSocketMatches**: BackendConfigPolicy/BackendTLSPolicy set `TransportSocket`, while Istio uses `TransportSocketMatches`

## References

- [PR #11297: plugins: export trafficpolicy merge, order plugin application](https://github.com/kgateway-dev/kgateway/pull/11297)
- [Design Document 11268: Policy Precedence and Application](design/11268.md)