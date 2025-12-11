@ -1,431 +0,0 @@
# Manual Testing Guide: FrontendTLSConfig (mTLS)

This guide provides step-by-step instructions for manually testing the `FrontendTLSConfig` feature in a Kubernetes cluster, including certificate generation and configuration examples.

## Overview

`FrontendTLSConfig` enables mutual TLS (mTLS) on Gateway listeners by configuring client certificate validation. This guide demonstrates:
- Default validation configuration (applies to all HTTPS listeners)
- Per-port validation configuration (overrides default for specific ports)
- Multiple CA certificate references support

## Prerequisites

- A Kubernetes cluster with kgateway installed
- `openssl` installed locally
- `kubectl` configured to access your cluster
- `curl` for testing

## Step 1: Generate Certificates

### 1.1 Generate CA Certificate (for validating client certificates)

```bash
# Create CA private key
openssl genrsa -out ca-key.pem 2048

# Create CA certificate (valid for 1 year)
openssl req -new -x509 -days 365 -key ca-key.pem -out ca-cert.pem \
  -subj "/CN=Test CA/O=Test Org"

# Optional: Generate a second CA certificate to test multiple CA refs
openssl genrsa -out ca2-key.pem 2048
openssl req -new -x509 -days 365 -key ca2-key.pem -out ca2-cert.pem \
  -subj "/CN=Test CA 2/O=Test Org"
```

### 1.2 Generate Server Certificate (for TLS termination)

```bash
# Create server private key
openssl genrsa -out server-key.pem 2048

# Create server certificate signing request
openssl req -new -key server-key.pem -out server.csr \
  -subj "/CN=example.com/O=Test Org"

# Create server certificate signed by CA (valid for 1 year)
openssl x509 -req -days 365 -in server.csr -CA ca-cert.pem -CAkey ca-key.pem \
  -CAcreateserial -out server-cert.pem \
  -extensions v3_req -extfile <(echo "[v3_req]"; echo "subjectAltName=DNS:example.com,DNS:*.example.com")
```

### 1.3 Generate Client Certificate (for mTLS client authentication)

```bash
# Create client private key
openssl genrsa -out client-key.pem 2048

# Create client certificate signing request
openssl req -new -key client-key.pem -out client.csr \
  -subj "/CN=client.example.com/O=Test Org"

# Create client certificate signed by CA (valid for 1 year)
openssl x509 -req -days 365 -in client.csr -CA ca-cert.pem -CAkey ca-key.pem \
  -CAcreateserial -out client-cert.pem
```

## Step 2: Create Kubernetes Resources

### 2.1 Create Namespace

```bash
kubectl create namespace test-mtls
```

### 2.2 Create Server TLS Secret

```bash
# Base64 encode server certificate and key
SERVER_CERT=$(cat server-cert.pem | base64 -w 0)
SERVER_KEY=$(cat server-key.pem | base64 -w 0)

# Create the secret
kubectl create secret tls https-cert \
  --cert=server-cert.pem \
  --key=server-key.pem \
  -n test-mtls
```

### 2.3 Create CA Certificate ConfigMaps

```bash
# Create ConfigMap with CA certificate (for default validation)
kubectl create configmap ca-cert-default \
  --from-file=ca.crt=ca-cert.pem \
  -n test-mtls

# Create ConfigMap with second CA certificate (for testing multiple refs)
kubectl create configmap ca-cert-default-2 \
  --from-file=ca.crt=ca2-cert.pem \
  -n test-mtls

# Create ConfigMap with CA certificate (for per-port validation)
kubectl create configmap ca-cert-per-port \
  --from-file=ca.crt=ca-cert.pem \
  -n test-mtls
```

### 2.4 Create Backend Service

```bash
# Create a simple httpbin service for testing
kubectl create deployment httpbin --image=kennethreitz/httpbin -n test-mtls
kubectl expose deployment httpbin --port=8000 --target-port=80 -n test-mtls
```

## Step 3: Configure Gateway with FrontendTLSConfig

### 3.1 Gateway with Per-Port Override

This configuration shows per-port validation overriding the default:

```bask
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: mtls-gateway-per-port
  namespace: test-mtls
spec:
  gatewayClassName: kgateway
  tls:
    frontend:
      default:
        validation:
          mode: AllowValidOnly
          caCertificateRefs:
            - name: ca-cert-default
              kind: ConfigMap
              group: ""
      perPort:
        - port: 8444
          tls:
            validation:
              mode: AllowInsecureFallback
              caCertificateRefs:
                - name: ca-cert-per-port
                  kind: ConfigMap
                  group: ""
  listeners:
  - name: https-default
    protocol: HTTPS
    port: 8443
    tls:
      mode: Terminate
      certificateRefs:
        - name: https-cert
          kind: Secret
  - name: https-per-port
    protocol: HTTPS
    port: 8444
    tls:
      mode: Terminate
      certificateRefs:
        - name: https-cert
          kind: Secret
EOF
```

### 3.2 Create HTTPRoute

```bash
# Apply HTTPRoute
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: mtls-route
  namespace: test-mtls
spec:
  parentRefs:
  - name: mtls-gateway-per-port
  hostnames:
  - "example.com"
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /
    backendRefs:
    - name: httpbin
      port: 8000
EOF
```

## Step 4: Get Gateway External IP/Port or Port Forward

```bash
# Get the Gateway service external IP
kubectl get svc -n kgateway-system | grep kgateway

# Or if using port-forward for testing:
kubectl port-forward -n test-mtls deploy/mtls-gateway-per-port  8443:8443 8444:8444 &
```

## Step 5: Test the Configuration

### 5.1 Test Without Client Certificate (Should Fail with AllowValidOnly)

```bash
# This should fail with SSL handshake error or 400 Bad Request
curl -v -k https://localhost:8443/get \
  --resolve example.com:8443:127.0.0.1 \
  -H "Host: example.com"
```

Expected: Connection failure or SSL handshake error

### 5.2 Test With Valid Client Certificate (Should Succeed)

```bash
# This should succeed
curl -v -k https://localhost:8443/get \
  --resolve example.com:8443:127.0.0.1 \
  -H "Host: example.com" \
  --cert client-cert.pem \
  --key client-key.pem \
  --cacert ca-cert.pem
```

Expected: HTTP 200 OK with httpbin response

### 5.3 Test Per-Port Override (Port 8444 with AllowInsecureFallback)

```bash
# Without client cert - should work with AllowInsecureFallback
curl -v -k https://localhost:8444/get \
  --resolve example.com:8444:127.0.0.1 \
  -H "Host: example.com"

# With client cert - should also work
curl -v -k https://localhost:8444/get \
  --resolve example.com:8444:127.0.0.1 \
  -H "Host: example.com" \
  --cert client-cert.pem \
  --key client-key.pem \
  --cacert ca-cert.pem
```

Expected: Both requests should succeed (HTTP 200 OK)


## Step 6: Verify Configuration

### 6.1 Check Gateway Status

```bash
kubectl get gateway mtls-gateway-per-port -n test-mtls -o yaml
```

Look for:
- `status.listeners[].conditions` - should show `Accepted: True`
- Any error conditions related to certificate references

### 6.2 Check Envoy Configuration (if accessible)

If you have access to Envoy admin interface or xDS dump:

```bash
# Port-forward to Envoy admin
kubectl port-forward -n test-mtls deployment/mtls-gateway-per-port 19000:19000

# Check listener configuration
curl http://localhost:19000/config_dump | jq '.configs[2].dynamic_listeners[] | select(.name | contains("8443"))'
```

Look for:
- `downstream_tls_context.require_client_certificate: true` (for AllowValidOnly)
- `downstream_tls_context.common_tls_context.validation_context.trusted_ca` (should contain CA cert)

### 6.3 Check Logs

```bash
# Check kgateway controller logs
kubectl logs -n kgateway-system -l app.kubernetes.io/name=kgateway --tail=100 | grep -i "frontend\|mtls\|client.*cert"

# Check Envoy logs
kubectl logs -n test-mtls -l app.kubernetes.io/name=mtls-gateway-per-port --tail=100 | grep -i "ssl\|tls\|certificate"
```

## Troubleshooting

### Issue: Connection refused

- Verify Gateway is listening on the expected port
- Check Gateway status for listener conditions
- Verify the Gateway service is exposing the correct ports

### Issue: Certificate validation errors

- Verify CA certificate ConfigMap contains valid PEM data under `ca.crt` key
- Check that client certificate is signed by one of the configured CA certificates
- Verify certificate hasn't expired

### Issue: Client certificate not required

- Verify `mode: AllowValidOnly` is set (not `AllowInsecureFallback`)
- Check that `caCertificateRefs` is properly configured
- Verify the Gateway resource was applied correctly


## Cleanup

```bash
# Delete resources
kubectl delete gateway mtls-gateway-per-port -n test-mtls
kubectl delete httproute mtls-route -n test-mtls
kubectl delete configmap ca-cert-default ca-cert-default-2 ca-cert-per-port -n test-mtls
kubectl delete secret https-cert -n test-mtls
kubectl delete svc httpbin -n test-mtls
kubectl delete deployment httpbin -n test-mtls
kubectl delete namespace test-mtls

# Clean up local certificate files
rm -f *.pem *.csr *.srl
```

## Additional Notes

- **AllowValidOnly**: Requires valid client certificate, rejects connections without one
- **AllowInsecureFallback**: Accepts connections with or without client certificates
- **Multiple CA refs**: All configured CA certificates are trusted, client certs signed by any of them are accepted
- **Per-port override**: Per-port configuration takes precedence over default configuration
