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

The Istio plugin can also configure TLS through:

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
- **Other backend object types**

## Conflict Resolution

### Current Implementation: No Plugin Ordering

**Important**: The current implementation has **no ordering enforcement** between backend/istio plugins.

#### How Backend Processing Works Today

Backend processing iterates over plugins using Go map iteration:

```go
for gk, policyPlugin := range t.ContributedPolicies {
    // Process each plugin
}
```

Since Go map iteration order is non-deterministic, the order in which TLS plugins are applied is **completely random**.

#### TLS Configuration Fields

**Important distinction**: Different plugins configure different Envoy fields:

- **BackendTLSPolicy** and **BackendConfigPolicy**: Set `TransportSocket` on the cluster
- **Istio plugin**: Sets `TransportSocketMatches` on the cluster

Since these are different fields, **BackendTLSPolicy/BackendConfigPolicy do NOT overwrite Istio's configuration** and vice versa.

#### Impact on TLS Configuration

1. **When auto-mTLS is enabled** (default): 
   - Istio sets `TransportSocketMatches` for automatic mTLS
   - BackendTLSPolicy/BackendConfigPolicy can still set `TransportSocket` (but this may conflict with Istio's behavior)
   - **Result: Potential conflicts between TransportSocket and TransportSocketMatches**

2. **When auto-mTLS is disabled** (via annotation):
   - Istio does not set `TransportSocketMatches`
   - BackendTLSPolicy/BackendConfigPolicy can set `TransportSocket` without conflicts
   - **Result: Clean separation, no conflicts**

3. **When multiple backend TLS plugins are present**:
   - BackendConfigPolicy overwrites BackendTLSPolicy's `TransportSocket` (last wins)
   - **Result: Unpredictable which backend TLS plugin wins**

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

## TLS Configuration Behavior

### Default Behavior (Istio Auto-mTLS Enabled)

When Istio is enabled and the `kgateway.dev/disable-istio-auto-mtls` annotation is either **not present** or set to anything other than `"true"`:

- **In-mesh backends** (Kubernetes Services): Istio automatically applies mTLS using TransportSocketMatches
- **External backends** (Backend resources): Traffic uses cleartext (no TLS)
- **BackendConfigPolicy and BackendTLSPolicy**: Can still set TransportSocket (may conflict with Istio)

### Custom TLS Behavior (Istio Auto-mTLS Disabled)

When the `kgateway.dev/disable-istio-auto-mtls: "true"` annotation is **present** on the backend resource:

- **Istio auto-mTLS**: Is disabled for this specific backend
- **BackendConfigPolicy**: Can configure TLS via TransportSocket
- **BackendTLSPolicy**: Can configure TLS via TransportSocket but will be overridden by BackendConfigPolicy if both are present
- **Plugin application order**: BackendTLSPolicy → BackendConfigPolicy (last wins for TransportSocket)
