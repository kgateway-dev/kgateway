# EP-12915: HTTP External Authorization Support

* Issue: [#12915](https://github.com/kgateway-dev/kgateway/issues/12915)

## Background

Kgateway currently supports external authorization (extauth) services exclusively via gRPC through the
`GatewayExtension` resource. This limitation prevents users from integrating with HTTP-based authorization services,
which are common in many authentication and authorization systems, like []. Envoy's `ext_authz` filter supports both
gRPC and HTTP protocols, but currently kgateway only exposes the gRPC option.

## Motivation

Many organizations deploy HTTP-based authorization services rather than gRPC services due to:
- Simpler integration with existing HTTP infrastructure
- Wider language/framework support for HTTP services
- Existing authorization services that only expose HTTP endpoints (e.g., OAuth2 Proxy, Authelia)

Adding HTTP support will enable these users to integrate with kgateway's external authorization without requiring a
protocol translation layer or rewriting their services.

## Goals

- Enable users to configure HTTP-based external authorization services via `GatewayExtension`
- Maintain backward compatibility with existing gRPC extauth configurations
- Support essential HTTP-specific features: path endpoints, header forwarding
- Support common configurations with gRPC: retry policy, timeout

## Non-Goals

- Deprecating or changing existing gRPC extauth functionality
- Supporting advanced Envoy ext_authz HTTP features (complex header matchers, multiple endpoints) in the initial implementation
- Automatic migration of gRPC configurations to HTTP

## Implementation Details

### API Changes

Extend `ExtAuthProvider` in `api/v1alpha1/ext_auth_types.go` to support both gRPC and HTTP services:

```go
type ExtAuthProvider struct {
    // Exactly one of grpcService or httpService must be set
    GrpcService *ExtGrpcService `json:"grpcService,omitempty"`
    HttpService *ExtHttpService `json:"httpService,omitempty"`
    // ... existing fields (FailOpen, ClearRouteCache, etc.)
	HeadersToForward []string `json:"headersToForward,omitempty"`
}

type ExtHttpService struct {
    BackendRef            gwv1.BackendRef          `json:"backendRef"`
    Path                  string                   `json:"path,omitempty"`
    RequestTimeout        *metav1.Duration         `json:"requestTimeout,omitempty"`
    AuthorizationRequest  *AuthorizationRequest    `json:"authorizationRequest,omitempty"`
    AuthorizationResponse *AuthorizationResponse   `json:"authorizationResponse,omitempty"`
    Retry                 *ExtAuthRetryPolicy      `json:"retry,omitempty"`
}

type AuthorizationRequest struct {
    HeadersToAdd map[string]string `json:"headersToAdd,omitempty"`
}

type AuthorizationResponse struct {
    HeadersToBackend []string `json:"headersToBackend,omitempty"`
}
```

**Key design decisions:**

CEL validation ensures exactly one of `grpcService` or `httpService` is specified.

1. **Path field**: HTTP requires explicit endpoint paths (e.g., `/authorize`, `/verify`) unlike gRPC where the method is in the protobuf definition
2. **AuthorizationRequest/Response**: Maps directly to Envoy's ext_authz proto structure, with `headersToAdd` for adding headers to the auth request and `headersToBackend` for forwarding headers from the auth response to upstream
3. **Retry policy**: Mirrors gRPC's retry and timeout configuration for consistency
4. **HeadersToForward**: Configured at `ExtAuthProvider` level (not HTTP service level) to specify which client headers to forward to the authorization service. By default, for HTTP services, Envoy forwards minimal headers (Method, Path, Host). This field enables forwarding critical headers like `cookie` and `authorization` that auth services need. Applies to both gRPC and HTTP services.

### Translation Layer

Add `ResolveExtHttpService` function in `internal/kgateway/extensions2/plugins/trafficpolicy/gateway_extension.go`:

- Resolves backend reference to cluster name
- Builds Envoy `HttpService` configuration with `HttpUri` and path prefix
- Translates `AuthorizationRequest.HeadersToAdd` to Envoy's `AuthorizationRequest.HeadersToAdd`
- Translates `AuthorizationResponse.HeadersToBackend` to Envoy's `AuthorizationResponse.AllowedUpstreamHeaders`
- Handles retry policy translation

Update `TranslateGatewayExtensionBuilder` to detect service type and build appropriate Envoy `ExtAuthz` config with
either `ExtAuthz_GrpcService` or `ExtAuthz_HttpService`.

### Configuration Example

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: GatewayExtension
metadata:
  name: http-extauth
spec:
  type: ExtAuth
  extAuth:
    headersToExtAuth:
      - cookie
      - authorization
    httpService:
      backendRef:
        name: auth-service
        port: 8080
      path: "/verify"
      requestTimeout: 500ms
      authorizationRequest:
        headersToAdd:
          x-forwarded-host: "gateway.example.com"
          x-request-id: "12345"
      authorizationResponse:
        headersToBackend:
          - x-user-id
          - x-auth-email
          - x-user-roles
      retry:
        attempts: 2
        backoff:
          baseInterval: 25ms
    failOpen: false
    statusOnError: 403
```

### Test Plan

- Translator tests verifying Envoy config generation
- E2E tests with HTTP-based extauth server

## Alternatives

### 1. Separate CRD for HTTP ExtAuth
Creating `HTTPExtAuthProvider` as a distinct type would avoid modifying existing types but leads to:
- Code duplication for shared fields (`failOpen`, `statusOnError`, etc.)
- User confusion about when to use which type
- More complex documentation

### 2. Full Envoy Feature Parity
Expose all Envoy `ext_authz` HTTP options (complex matchers, multiple auth servers), but:
- Increases API complexity significantly
- Most users need simple path + header forwarding
- Can be added incrementally if needed

## Open Questions

- Any other configurations that should be supported for HTTP ExtAuth?
