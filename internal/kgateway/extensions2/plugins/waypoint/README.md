# Kgateway Istio Waypoint Integration

**Note:** This document serves as a reference for some of key design decisions related to Kgateway Istio Waypoint integration. Rather than creating extensive documentation for each decision, this file collects important decision points to provide a centralized reference.

## Authorization Policy Namespace Configuration

### Overview
This directory contains the implementation for integrating Kgateway with Istio Waypoints, including support for waypoint authorization policies through GatewayClass as a TargetRef.

### Root Namespace for Authorization Policies
When implementing GatewayClass as a TargetRef for waypoint authorization policies, we needed to determine which namespace(s) should be used for global policies. After discussion among maintainers, the following decisions were made:

1. **Default Root Namespace**: The default root namespace for waypoint authorization policies will be `istio-system` to maintain consistency with Istio's implementation.

2. **Configurability**: The root namespace is configurable through Settings, allowing users to change it if needed.

3. **Setting Name**: The configuration setting will be called "Mesh root namespace" to clearly indicate its purpose.

### Rationale
- Maintaining consistency with Istio's implementation reduces confusion for users, especially during migration.
- The `istio-system` namespace is typically the permanent root namespace for mesh configurations.
- Waypoint configurations should be consistent with ztunnel regardless of whether using Istio, Gloo, or KGateway.
- Having a single configurable namespace (rather than multiple namespaces) avoids potential conflicts between duplicate authorization policies.

### Usage
Authorization policies targeting waypoints should be placed in the `istio-system` namespace by default, or in the configured mesh root namespace if changed via Settings.

```yaml
# Example authorization policy in istio-system namespace
apiVersion: security.istio.io/v1beta1
kind: AuthorizationPolicy
metadata:
  name: waypoint-policy
  namespace: istio-system
spec:
  targetRef:
    kind: GatewayClass
    name: kgateway-waypoint
  # Policy details...
```

For users migrating from Istio to KGateway, using the default `istio-system` namespace simplifies the transition by allowing existing authorization policies to work with KGateway waypoints.