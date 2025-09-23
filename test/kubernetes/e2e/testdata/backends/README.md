# Standardized Backend Services for E2E Tests

This directory contains standardized backend service manifests for kgateway e2e tests.

## Available Backends

### 1. httpbin (Recommended for HTTP testing)
- **httpbin.yaml**: Default namespace deployment
- **httpbin-namespaced.yaml**: For custom namespace deployments
- **Size**: ~10MB container image (go-httpbin)
- **Use cases**: HTTP testing, request inspection, general API testing

### 2. nginx-tls (Only for TLS testing)
- **nginx-tls.yaml**: Default namespace deployment with TLS
- **nginx-tls-namespaced.yaml**: For custom namespace TLS deployments
- **Size**: ~140MB container image (nginx:1.25-alpine)
- **Use cases**: TLS/SSL backend testing, certificate validation

## Usage Guidelines

### When to use httpbin
- ✅ General HTTP testing
- ✅ Request/response validation
- ✅ Header inspection
- ✅ JSON response testing
- ✅ Lighter resource usage

### When to use nginx-tls
- ✅ TLS/SSL backend testing only
- ✅ Certificate validation tests
- ⚠️ Use sparingly due to larger image size

## Test Helper Functions

```go
// Apply standard httpbin backend
func ApplyHTTPBinBackend(ctx context.Context, client client.Client, namespace string) error {
    // Implementation would use httpbin.yaml or httpbin-namespaced.yaml
}

// Apply TLS-enabled nginx backend (use only when TLS is required)
func ApplyNginxTLSBackend(ctx context.Context, client client.Client, namespace string) error {
    // Implementation would use nginx-tls.yaml or nginx-tls-namespaced.yaml
}

// Cleanup backend resources
func CleanupBackend(ctx context.Context, client client.Client, name, namespace string) error {
    // Implementation would remove all backend resources
}
```

## Service Details

### HTTPBin Services
- **HTTP Port**: 8000
- **Endpoints**:
  - `/` - Basic response
  - `/json` - JSON response
  - `/headers` - Request headers echo
  - `/get`, `/post`, `/put`, `/delete` - HTTP method testing
  - `/status/{code}` - HTTP status code testing

### Nginx TLS Services
- **HTTP Port**: 8080 (targetPort 80)
- **HTTPS Port**: 8443 (targetPort 443)
- **TLS**: Self-signed certificate for `example.com`
- **Endpoints**:
  - `/` - Simple OK response
  - `/health` - Health check endpoint

## Migration Path

1. **Identify test requirements**: Does your test need TLS? Use nginx-tls. Otherwise use httpbin.
2. **Update manifests**: Replace existing nginx deployments with standardized templates
3. **Update test code**: Use helper functions instead of inline manifests
4. **Resource optimization**: Remove unused nginx deployments to save CI resources

## Benefits

- **Consistency**: All tests use the same backend configurations
- **Performance**: Lighter containers (httpbin) for most use cases
- **Maintenance**: Centralized backend definitions
- **Resource efficiency**: Reduced image pulls and memory usage in CI

## Certificate Details (nginx-tls only)

The included self-signed certificate is valid for:
- **Subject**: example.com
- **Valid**: 2024-07-02 to 2025-08-01
- **Protocols**: TLS 1.2, TLS 1.3
- **Usage**: Testing only - not for production