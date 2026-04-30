package gatewaytls

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/sslutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestResolveBackendClientCertificate(t *testing.T) {
	t.Run("resolves a valid secret ref", func(t *testing.T) {
		gateway := testGatewayWithClientCertificateRef(gwv1.SecretObjectReference{
			Name: "client-cert",
		})

		resolved, err := ResolveBackendClientCertificate(gateway, func(secretRef gwv1.SecretObjectReference) (*ir.Secret, error) {
			assert.Equal(t, gwv1.ObjectName("client-cert"), secretRef.Name)
			return &ir.Secret{
				ObjectSource: ir.ObjectSource{
					Namespace: "default",
					Name:      "client-cert",
				},
				Data: map[string][]byte{
					corev1.TLSCertKey:       []byte(testTLSCert),
					corev1.TLSPrivateKeyKey: []byte(testTLSKey),
				},
			}, nil
		})

		require.NoError(t, err)
		require.NotNil(t, resolved)
		assert.Equal(t, testTLSKey, string(resolved.Certificate.PrivateKey))
		assert.NotEmpty(t, resolved.Certificate.CertChain)
	})

	t.Run("rejects non-secret refs", func(t *testing.T) {
		gateway := testGatewayWithClientCertificateRef(gwv1.SecretObjectReference{
			Kind: ptr.To(gwv1.Kind("ConfigMap")),
			Name: "client-cert",
		})

		called := false
		resolved, err := ResolveBackendClientCertificate(gateway, func(secretRef gwv1.SecretObjectReference) (*ir.Secret, error) {
			called = true
			return nil, nil
		})

		require.Error(t, err)
		assert.Nil(t, resolved)
		assert.False(t, called)
		assert.Contains(t, err.Error(), "only core Secret references are supported")
	})

	t.Run("rejects secrets missing the private key", func(t *testing.T) {
		gateway := testGatewayWithClientCertificateRef(gwv1.SecretObjectReference{
			Name: "client-cert",
		})

		resolved, err := ResolveBackendClientCertificate(gateway, func(secretRef gwv1.SecretObjectReference) (*ir.Secret, error) {
			return &ir.Secret{
				ObjectSource: ir.ObjectSource{
					Namespace: "default",
					Name:      "client-cert",
				},
				Data: map[string][]byte{
					corev1.TLSCertKey: []byte(testTLSCert),
				},
			}, nil
		})

		require.Error(t, err)
		assert.Nil(t, resolved)
		assert.True(t, errors.Is(err, sslutils.ErrInvalidTlsSecret))
	})
}

func testGatewayWithClientCertificateRef(ref gwv1.SecretObjectReference) *ir.Gateway {
	return &ir.Gateway{
		ObjectSource: ir.ObjectSource{
			Group:     gwv1.GroupVersion.Group,
			Kind:      "Gateway",
			Namespace: "default",
			Name:      "gw",
		},
		BackendTLSConfig: &ir.GatewayBackendTLSConfigIR{
			ClientCertificateRef: ref.DeepCopy(),
		},
	}
}

// openssl req -x509 -newkey rsa:2048 -keyout test.key -out test.crt -days 365 -nodes -subj "/CN=test.example.com" -addext "subjectAltName=DNS:test.example.com"
const testTLSCert = `-----BEGIN CERTIFICATE-----
MIIDNDCCAhygAwIBAgIUL6jJHHVicPbTrxNXjTX2ti/2swgwDQYJKoZIhvcNAQEL
BQAwGzEZMBcGA1UEAwwQdGVzdC5leGFtcGxlLmNvbTAeFw0yNTA1MjkxNTQ5MTha
Fw0yNjA1MjkxNTQ5MThaMBsxGTAXBgNVBAMMEHRlc3QuZXhhbXBsZS5jb20wggEi
MA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQC0zE9AuZN4Uc5VOsUbLYHZaEh/
db2HiHsYdyxpuLx1C2aXYUpIyGjwVSs84+TwS46XRCstZHsTDrSvlM6hwU2x+B7E
FksEM5TPU/0e6+lUde0yUweiYCYIKnJU1PzWO7pldS8K8ayvTYbIMSWawzCgWeq1
OWPgwCfSK0GF2MyfhfAqMYazZB9rZhGycyaaE1iKX97JyYU79klhnaEdZE3bhCNr
wH2s5h55jbIrizUAbjz6+t5B+euakUrfKCeGXfCb3TNz48IEWdNIMPmyfgSWzXlz
MXKpfZ0tza6SzeqrDLZN2nl/YydM1yHmI7MALrIXJo0hXk4N469f/MIdCKZdAgMB
AAGjcDBuMB0GA1UdDgQWBBS1oJXQN8/QuWWlo+UfZe2SKxy2ezAfBgNVHSMEGDAW
gBS1oJXQN8/QuWWlo+UfZe2SKxy2ezAPBgNVHRMBAf8EBTADAQH/MBsGA1UdEQQU
MBKCEHRlc3QuZXhhbXBsZS5jb20wDQYJKoZIhvcNAQELBQADggEBAFtjff8nA/+I
2vLVq6SE3eLe/x4w09RtpdNZ+qirAQsbu0DrI1F9/MNxSYhKMA+4DCj1OXpUaaPO
mwZIwEtFklUyDqz8aaBK8xCBjzvc++rbaiY2XVDo+/e6ML0c90LXyGI3pDK6bUU1
15dFeYikl+7iVf4L+DrWgj7imK5LtWqKS7VTUX/+yFnA19d7LJF2/uOnprIeEHsj
LSlVx4yPJjGQYighFyK6VQKi3rsiuFU/LsedNEq2kJonn/NfT9pCvoReQqjijlyS
D8sD7wlIiyowZO09KIU7MUfPUqGlGsNXQ9Hy9sHJgPmsz4ZM4NofSOdt8MGETulJ
Tr8dXUTlbn0=
-----END CERTIFICATE-----
`

const testTLSKey = `-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC0zE9AuZN4Uc5V
OsUbLYHZaEh/db2HiHsYdyxpuLx1C2aXYUpIyGjwVSs84+TwS46XRCstZHsTDrSv
lM6hwU2x+B7EFksEM5TPU/0e6+lUde0yUweiYCYIKnJU1PzWO7pldS8K8ayvTYbI
MSWawzCgWeq1OWPgwCfSK0GF2MyfhfAqMYazZB9rZhGycyaaE1iKX97JyYU79klh
naEdZE3bhCNrwH2s5h55jbIrizUAbjz6+t5B+euakUrfKCeGXfCb3TNz48IEWdNI
MPmyfgSWzXlzMXKpfZ0tza6SzeqrDLZN2nl/YydM1yHmI7MALrIXJo0hXk4N469f
/MIdCKZdAgMBAAECggEAFLPqhVZauSXg8yiCJo0M9+i1mIbSd6Ecu132I3sIdXyj
OEVnPLNaNN8Dzvqnng6A2vhu20lMwI9oCE0JZkNc0rq/RyPoXihL63vKGc7Yzpec
XC1ey+ynnjrCEc270ApR20lSZDXtWLuPagAatsCQImR5eFwEgFlwlePnIl0DfWan
JQQYf5hbayLXwcoaDXxCB8rmkGpwBsamYVDjLgxjxmQwjMf809jWi16OM6mIgXot
H4ZowMj26HbKhBZqpM85hzliHNAsNuCnfSQJGSeJzMvJR/UBRnnofDPKhCdeoIMt
7iu5uMMd42h1tYIgk9KFw3S24G3GjRYIb3VpqfaMPwKBgQDa1iZA1BLb2rzWayrE
Tq43dMM9n3seMOx12VaA9MPfGMJh/uJEgQXH8MxvbsRhTw2IUzT4eJ/3SxHQF1ru
G8421IZQPShE3/1J1vRngE9EUMKfO8fLKVIM7VzugFTBN4HB/raDbtH6go2Qg/t+
UDzFLv1qt3Mjmbluwvr2Cw0kLwKBgQDTgHNm53VSHUyrgVkAl6BvFO6S07pNsKIe
LCWcIIXDLjat7kYgnaMXkmNuSsfPesYeq9kyLPh4YYUTJlSpWZPkm+8NtjPSLwa2
phxX5AIiC/ZZEutTkQy+a3KERjE5sW4dJbjeFXNjqO4f1v/L03hcfaTeFK6zkz0v
LpJhXpNfMwKBgQCVcVcQQINcdoUs3GSJSL36ixdltspqNLjWRgSn7f7xFMRyDZDR
fVbIUq4Zjwg297hjF4d+A0oio7ZXaAulvYFWuk267/jXCCu9yDiBkgMPwSMXgMiQ
+ffZciNbkHHQvSo0o9BZ800cCRnJzgfqG7tUYSGYRg0wC6Oxex/M9IEV6wKBgD8t
B0udp7W3esdgA63hnNKRdhH1nJjIQiSxGyrfrBT5IOwjWF81txm7aGfxfm3DRpqy
ylXqiO2sc4ucz30mfL60tVtrKV+HHIJCbAT03o489ID23cRAd4YJolNQhDOvhCzA
r8/mqGkEdNyd5BqGOFWoUi7kDqslOAl359Gd5ndxAoGAK3TVwhuLR9XoicDjmo6b
6qtYp/ln1Sx61ERo4Vaz/EdMCeVBD/DH3g0trdjI6XgFBJjuMvrz6LaRpvPkIxul
8VsYXhVwMPnyJzEd+wpEsIgIh5W9YluY0f5TxqcQkRGPUW5Sb5dXuk9BXRtaBzR0
35NY368cSzjvlBCisA91TbY=
-----END PRIVATE KEY-----
`
