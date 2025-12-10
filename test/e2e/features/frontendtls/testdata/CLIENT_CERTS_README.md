# Client Certificates for mTLS Testing

This directory contains client certificates used for mutual TLS (mTLS) testing with the `verify-certificate-hash` annotation and `FrontendTLSConfig`.

## Directory Structure

Certificates are organized into subdirectories:

- **`certs/ca1/`** - CA certificate 1 and its related client certificates:
  - `ca-cert-configmap.yaml` - CA certificate 1 (ConfigMap)
  - `client-cert-secret.yaml` - Client certificate for FrontendTLSConfig tests (signed by CA 1)
  - `client-certs-8443-9443-secret.yaml` - Client certificates for ports 8443 and 9443 (signed by CA 1)

- **`certs/ca2/`** - CA certificate 2 and its related client certificates:
  - `ca-cert-2-configmap.yaml` - CA certificate 2 (ConfigMap)
  - `client-cert-2-secret.yaml` - Client certificate for multiple CA tests (signed by CA 2)

**Note**: Tests only use the Kubernetes secrets (YAML manifests) which contain base64-encoded certificates. Raw certificate files (`.crt`, `.key`, `.pem`) are not present in this repository - they are artifacts from certificate generation that can be regenerated using the commands in the Certificate Generation section below.

**Note**: During certificate generation, intermediate files like `.csr` (Certificate Signing Request) and `.srl` (serial number) files may be created. These are temporary artifacts and are not committed to the repository - only the final certificates in the YAML manifests are required.

**Note**: The server TLS certificate (`tls-secret.yaml`) is located at the testdata root level alongside other Kubernetes manifests like `gw.yaml` and `curl-pod-with-certs.yaml`.

These certificates are signed by the CA certificates in `certs/ca1/ca-cert-configmap.yaml` and `certs/ca2/ca-cert-2-configmap.yaml`, allowing them to work with both validation mechanisms:
- `verify-certificate-hash`: Validates the specific certificate hash (certificate pinning)
- `FrontendTLSConfig`: Validates the certificate chain against the CA

## Certificate Generation

### Step 1: Generate CA Certificate and Key
```bash
openssl genrsa -out ca-key.pem 2048
openssl req -new -x509 -days 365 -key ca-key.pem -out ca-cert.pem \
  -subj "/CN=Test CA/O=Test Org"
```

### Step 2: Generate Client Certificate for Port 8443
```bash
openssl genrsa -out client-8443.key 2048
openssl req -new -key client-8443.key -out client-8443.csr \
  -subj "/C=US/ST=California/L=San Francisco/O=Client Inc./CN=client.example.com"
openssl x509 -req -days 3650 -in client-8443.csr -CA ca-cert.pem -CAkey ca-key.pem \
  -CAcreateserial -out client-8443.crt
```

### Step 3: Generate Client Certificate for Port 9443
```bash
openssl genrsa -out client-9443.key 2048
openssl req -new -key client-9443.key -out client-9443.csr \
  -subj "/C=US/ST=California/L=San Francisco/O=Client Inc./CN=client-alt.example.com"
openssl x509 -req -days 3650 -in client-9443.csr -CA ca-cert.pem -CAkey ca-key.pem \
  -CAcreateserial -out client-9443.crt
```

### Step 4: Generate Client Certificate for FrontendTLSConfig Tests (Ports 8444/8445)
```bash
openssl genrsa -out client-frontend.key 2048
openssl req -new -key client-frontend.key -out client-frontend.csr \
  -subj "/CN=client.example.com/O=Test Org"
openssl x509 -req -days 3650 -in client-frontend.csr -CA ca-cert.pem -CAkey ca-key.pem \
  -CAcreateserial -out client-frontend.crt
# Base64 encode and update certs/ca1/client-cert-secret.yaml:
base64 -i client-frontend.crt | tr -d '\n'
base64 -i client-frontend.key | tr -d '\n'
```

### Step 5: Generate Second CA Certificate for Multiple CA Tests
```bash
# Generate second CA certificate and key
openssl genrsa -out ca2-key.pem 2048
openssl req -new -x509 -days 365 -key ca2-key.pem -out ca2-cert.pem \
  -subj "/CN=Test CA 2/O=Test Org"
# Base64 encode and update certs/ca2/ca-cert-2-configmap.yaml:
base64 -i ca2-cert.pem | tr -d '\n'
```

### Step 6: Generate Client Certificate Signed by Second CA (Port 8446)
```bash
# Generate client certificate signed by CA 2
openssl genrsa -out client2-key.pem 2048
openssl req -new -key client2-key.pem -out client2.csr \
  -subj "/CN=client2.example.com/O=Test Org"
openssl x509 -req -days 3650 -in client2.csr -CA ca2-cert.pem -CAkey ca2-key.pem \
  -CAcreateserial -out client2-cert.pem
# Base64 encode and update certs/ca2/client-cert-2-secret.yaml:
base64 -i client2-cert.pem | tr -d '\n'
base64 -i client2-key.pem | tr -d '\n'
```

### SHA256 Fingerprints
To calculate the SHA256 fingerprint (used in `verify-certificate-hash` annotation):
```bash
# For port 8443 certificate
openssl x509 -in client-8443.crt -noout -fingerprint -sha256 | cut -d= -f2 | \
  tr -d ':' | awk '{for(i=1;i<=length($0);i+=2) printf "%s:", substr($0,i,2); print ""}' | \
  sed 's/:$//' | tr '[:lower:]' '[:upper:]'

# For port 9443 certificate
openssl x509 -in client-9443.crt -noout -fingerprint -sha256 | cut -d= -f2 | \
  tr -d ':' | awk '{for(i=1;i<=length($0);i+=2) printf "%s:", substr($0,i,2); print ""}' | \
  sed 's/:$//' | tr '[:lower:]' '[:upper:]'
```

**Calculated hashes:**
- **Port 8443 cert**: `59:CF:81:16:72:71:C7:17:23:4E:CE:4F:F9:B9:68:B5:B0:40:2A:00:BB:14:64:E0:42:B9:AD:DA:61:C7:03:F0`
- **Port 9443 cert**: `E8:44:3B:AA:87:7C:DD:71:31:59:02:77:A4:2F:20:C6:B6:F5:7B:02:29:07:E0:F7:8C:21:DE:41:5B:28:CF:BB`

## Usage in Tests

### Test File Mapping
In the tests, these certificates are mounted into the curl pod from Kubernetes secrets at:
- `/etc/client-certs/client-8443.crt` / `/etc/client-certs/client-8443.key` - Certificate valid for port 8443 listener (from `client-certs` secret, source: `certs/ca1/client-certs-8443-9443-secret.yaml`)
- `/etc/client-certs/client-9443.crt` / `/etc/client-certs/client-9443.key` - Certificate valid for port 9443 listener (from `client-certs` secret, source: `certs/ca1/client-certs-8443-9443-secret.yaml`)
- `/etc/client-certs-frontend/tls.crt` / `/etc/client-certs-frontend/tls.key` - Certificate valid for FrontendTLSConfig tests on ports 8444/8445 (from `client-cert` secret, source: `certs/ca1/client-cert-secret.yaml`, signed by CA 1)
- `/etc/client-certs-2-frontend/tls.crt` / `/etc/client-certs-2-frontend/tls.key` - Certificate valid for multiple CA tests on port 8446 (from `client-cert-2` secret, source: `certs/ca2/client-cert-2-secret.yaml`, signed by CA 2)

### Gateway Configuration
- **Port 8443** (mtls.example.com): `verify-certificate-hash` = SHA256 of the port 8443 certificate + default FrontendTLSConfig (AllowInsecureFallback)
- **Port 9443** (mtls-alt.example.com): `verify-certificate-hash` = SHA256 of the port 9443 certificate + default FrontendTLSConfig (AllowInsecureFallback)
- **Port 8444** (example.com): FrontendTLSConfig per-port (AllowInsecureFallback) - client cert optional
- **Port 8445** (example.com): FrontendTLSConfig per-port (AllowValidOnly) - client cert required
- **Port 8446** (*.example.com): FrontendTLSConfig per-port (AllowValidOnly) with **multiple CA cert refs** (ca-cert and ca-cert-2) for wildcard domain - client cert required, accepts certs signed by either CA

### Test Validation

#### verify-certificate-hash Tests (TestVerifyCertificateHash)
The test suite uses these certificates with curl's `--cert` and `--key` flags to validate:
1. Connections succeed when the client cert hash matches the configured hash on each listener
2. Connections fail when the client cert hash doesn't match (cross-validation between ports)
3. Connections fail when no client cert is provided to an mTLS-enabled listener
4. The regular listener (port 443) works without client certificates

#### FrontendTLSConfig Tests (TestFrontendTLSConfig)
The test suite validates FrontendTLSConfig behavior:
1. Port 8445 (AllowValidOnly): Connections fail without client cert (exit code 16 - SSL error)
2. Port 8445 (AllowValidOnly): Connections succeed with valid client cert signed by CA
3. Port 8444 (AllowInsecureFallback): Connections succeed without client cert
4. Port 8444 (AllowInsecureFallback): Connections succeed with valid client cert signed by CA

#### Multiple CA Certificates Tests (TestMultipleCACertificates)
The test suite validates that FrontendTLSConfig supports multiple CA certificate references for wildcard domains (as specified in issue #12938):
1. Port 8446 (AllowValidOnly with multiple CA refs, wildcard domain *.example.com): Connections succeed with client cert signed by first CA (ca-cert) when accessing test.example.com
2. Port 8446 (AllowValidOnly with multiple CA refs, wildcard domain *.example.com): Connections succeed with client cert signed by second CA (ca-cert-2) when accessing test.example.com
3. Port 8446 (AllowValidOnly with multiple CA refs, wildcard domain *.example.com): Connections fail without client cert (exit code 16 - SSL error)

**Note**: Both `verify-certificate-hash` and `FrontendTLSConfig` can be used together on the same listener. The `verify-certificate-hash` validates the specific certificate hash (certificate pinning), while `FrontendTLSConfig` validates the certificate chain against the CA.

**Multiple CA Support**: FrontendTLSConfig supports multiple CA certificate references in the `caCertificateRefs` array. When multiple CAs are configured, client certificates signed by any of the configured CAs will be accepted. This is useful for scenarios where you need to support clients with certificates from different certificate authorities (e.g., during CA migration or supporting multiple client organizations). This feature works with wildcard domains, allowing multiple root CA certs for the same wildcard domain pattern (e.g., `*.example.com`).

