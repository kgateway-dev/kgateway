# Standardization Plan: Backend Test Services in kgateway E2E Tests

## Executive Summary

Replace nginx with lighter-weight alternatives while maintaining TLS testing capabilities. Standardize on httpbin for most HTTP testing and create a minimal TLS-enabled service for backend TLS tests.

## Motivation

1. **Performance**: nginx containers are heavier (~140MB) vs httpbin (~10MB)
2. **Consistency**: Multiple nginx manifests with similar but inconsistent configurations
3. **Purpose-built**: httpbin is designed for HTTP testing with better introspection capabilities
4. **Maintenance**: Fewer custom certificates and complex nginx configurations to maintain

## Proposed Changes

### 1. Primary Backend Service: httpbin

**Use for:**
- Basic HTTP routing tests
- Header manipulation tests  
- Response code testing
- Request/response inspection
- Most gateway functionality tests

**Benefits:**
- Lightweight (10MB vs 140MB for nginx)
- Built-in JSON response format for easy assertions
- Rich set of endpoints (/get, /post, /headers, /status/*, etc.)
- No custom configuration needed
- Already used in some tests

### 2. TLS Backend Service: lightweight-nginx

**Use for:**
- Backend TLS policy testing
- mTLS testing
- SSL/TLS certificate validation

**Benefits:**
- Minimal configuration focused only on TLS
- Consistent certificates across all TLS tests
- Smaller surface area than full nginx

### 3. Standardized Manifests

Create three standard manifest templates:

1. **`test/kubernetes/e2e/testdata/backends/httpbin.yaml`**
   - Standard httpbin deployment
   - Consistent labels and service configuration
   - Default namespace deployment

2. **`test/kubernetes/e2e/testdata/backends/httpbin-namespaced.yaml`**
   - Same as above but with dedicated namespace
   - For tests requiring namespace isolation

3. **`test/kubernetes/e2e/testdata/backends/nginx-tls.yaml`**
   - Minimal nginx with TLS configuration
   - Standard certificates for testing
   - Only for TLS-specific tests

## Implementation Plan

### Phase 1: Create Standard Manifests
- [ ] Create standardized httpbin manifest
- [ ] Create minimal TLS nginx manifest
- [ ] Generate consistent test certificates
- [ ] Add validation helper functions

### Phase 2: Migrate Tests by Category
- [ ] Migrate basic routing tests to httpbin
- [ ] Migrate backend config tests to httpbin  
- [ ] Keep TLS tests on minimal nginx
- [ ] Update test assertions for httpbin responses

### Phase 3: Cleanup
- [ ] Remove duplicate nginx manifests
- [ ] Update test documentation
- [ ] Verify no regression in test coverage

## Migration Map

| Test Suite | Current Backend | Proposed Backend | Rationale |
|------------|----------------|------------------|-----------|
| basicrouting | nginx | httpbin | Simple HTTP testing |
| backends | nginx | httpbin | Response validation easier |
| backendconfigpolicy | nginx | httpbin | HTTP-focused testing |
| services/httproute | nginx | httpbin | Route validation |
| metrics | nginx | httpbin | Lighter weight |
| backendtls | nginx | nginx-tls (minimal) | TLS required |
| istio (TLS tests) | nginx | nginx-tls (minimal) | mTLS required |

## Benefits

1. **Faster test execution**: Smaller images = faster pod startup
2. **Better assertions**: httpbin JSON responses easier to validate
3. **Reduced complexity**: Fewer custom nginx configurations
4. **Maintained TLS coverage**: Keep TLS testing with minimal nginx
5. **Consistency**: Standardized manifests across all tests