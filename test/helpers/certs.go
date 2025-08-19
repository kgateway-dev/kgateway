//go:build ignore

package helpers

// from here: https://golang.org/src/crypto/tls/generate_cert.go

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"

	"golang.org/x/crypto/ocsp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func publicKey(priv interface{}) interface{} {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	default:
		return nil
	}

}

func pemBlockForKey(priv interface{}) *pem.Block {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to marshal ECDSA private key: %v", err)
			os.Exit(2)
		}
		return &pem.Block{Type: "EC PRIVATE KEY", Bytes: b}
	default:
		return nil
	}

}

// Params includes parameters used to generate an x509 certificate.
type Params struct {
	Hosts            string             // Comma-separated hostnames and IPs to generate a certificate for
	ValidFrom        *time.Time         // Creation date
	ValidFor         *time.Duration     // Duration that certificate is valid for
	IsCA             bool               // whether this cert should be its own Certificate Authority
	RsaBits          int                // Size of RSA key to generate. Ignored if EcdsaCurve is set
	EcdsaCurve       string             // ECDSA curve to use to generate a key. Valid values are P224, P256 (recommended), P384, P521
	AdditionalUsages []x509.ExtKeyUsage // Usages to define in addition to default x509.ExtKeyUsageServerAuth
	IssuerKey        interface{}        // If provided, the certificate will be signed by this key
}

// GetCerts generates a signed key and certificate for the given parameters.
// If an IssuerKey is provided, the certificate will be signed by that key. Otherwise, a self-signed certificate will be generated.
func GetCerts(params Params) (string, string) {

	if len(params.Hosts) == 0 {
		ginkgo.Fail("Missing required --host parameter")
	}

	var priv interface{}
	// If an issuer key is provided, use it to sign the certificate
	if params.IssuerKey != nil {
		priv = params.IssuerKey
	} else {
		var err error
		switch params.EcdsaCurve {
		case "":
			if params.RsaBits == 0 {
				params.RsaBits = 2048
			}
			priv, err = rsa.GenerateKey(rand.Reader, params.RsaBits)
		case "P224":
			priv, err = ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
		case "P256":
			priv, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		case "P384":
			priv, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		case "P521":
			priv, err = ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
		default:
			ginkgo.Fail(fmt.Sprintf("Unrecognized elliptic curve: %q", params.EcdsaCurve))
		}
		Expect(err).NotTo(HaveOccurred())
	}

	var notBefore time.Time
	if params.ValidFrom == nil {
		notBefore = time.Now().Add(-time.Minute)
	} else {
		notBefore = *params.ValidFrom
	}

	if params.ValidFor == nil {
		tmp := time.Hour * 24
		params.ValidFor = &tmp
	}

	notAfter := notBefore.Add(*params.ValidFor)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		ginkgo.Fail(fmt.Sprintf("failed to generate serial number: %s", err))
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	template.ExtKeyUsage = append(template.ExtKeyUsage, params.AdditionalUsages...)

	hosts := strings.Split(params.Hosts, ",")
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	if params.IsCA {
		template.IsCA = true
		template.KeyUsage |= x509.KeyUsageCertSign
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey(priv), priv)
	if err != nil {
		ginkgo.Fail(fmt.Sprintf("Failed to create certificate: %s", err))
	}

	var certOut bytes.Buffer
	err = pem.Encode(&certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	Expect(err).NotTo(HaveOccurred())

	var keyOut bytes.Buffer

	err = pem.Encode(&keyOut, pemBlockForKey(priv))
	Expect(err).NotTo(HaveOccurred())

	return certOut.String(), keyOut.String()
}

var (
	getCerts     sync.Once // Used to generate CA certs for the proxy once.
	getMtlsCerts sync.Once // Used to generate mTLS CA certs for the proxy once.
	mtlsCert     string    // mTLS CA certificate for the proxy.
	mtlsPrivKey  string    // mTLS CA private key for the proxy.
	cert         string    // CA certificate for the proxy.
	privKey      string    // CA private key for the proxy.
)

// gencerts generates CA certs for the proxy.
func gencerts() {
	cert, privKey = GetCerts(Params{
		Hosts: "gateway-proxy,knative-proxy,ingress-proxy",
		IsCA:  true,
	})
}

// genmtlscerts generates mTLS certs for the proxy.
func genmtlscerts() {
	mtlsCert, mtlsPrivKey = GetCerts(Params{
		Hosts:            "gateway-proxy,knative-proxy,ingress-proxy",
		IsCA:             true,
		AdditionalUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
}

// Certificate returns the CA certificate for the proxy.
func Certificate() string {
	getCerts.Do(gencerts)
	return cert
}

// PrivateKey returns the CA private key for the proxy.
func PrivateKey() string {
	getCerts.Do(gencerts)
	return privKey
}

// MtlsCertificate returns an mTLS CA certificate for the proxy.
func MtlsCertificate() string {
	getMtlsCerts.Do(genmtlscerts)
	return mtlsCert
}

// MtlsPrivateKey returns an mTLS CA private key for the proxy.
func MtlsPrivateKey() string {
	getMtlsCerts.Do(genmtlscerts)
	return mtlsPrivKey
}

func GetKubeSecret(name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       []byte(Certificate()),
			corev1.TLSPrivateKeyKey: []byte(PrivateKey()),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// GetCertificateFromString returns an x509 certificate from the certificate's string representation.
func GetCertificateFromString(certificate string) *x509.Certificate {
	block, _ := pem.Decode([]byte(certificate))
	cert, err := x509.ParseCertificate(block.Bytes)
	Expect(err).NotTo(HaveOccurred())
	return cert
}

// GetPrivateKeyRSAFromString returns an RSA private key from the key's string representation.
func GetPrivateKeyRSAFromString(privateKey string) *rsa.PrivateKey {
	block, _ := pem.Decode([]byte(privateKey))
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	Expect(err).NotTo(HaveOccurred())
	return key
}

// FakeOcspResponder is a fake OCSP responder that can be used to generate OCSP responses.
type FakeOcspResponder struct {
	// The certificate of the OCSP responder.
	certificate *x509.Certificate
	// The private key of the OCSP responder.
	privateKey *rsa.PrivateKey
	// The certificate of the CA that signed the OCSP responder's certificate.
	// It is assumed that the signer has also signed any certificate that FakeOcspResponder will be generating responses for.
	issuer *x509.Certificate
}

// NewFakeOcspResponder creates a new fake OCSP responder from the given root CA.
func NewFakeOcspResponder(rootCa *x509.Certificate, rootKey interface{}) *FakeOcspResponder {
	// Generate a certificate for the OCSP responder, signed by the root key passed.
	cert, key := GetCerts(Params{
		Hosts:     "ocsp-responder",
		IsCA:      false,
		IssuerKey: rootKey,
	})

	return &FakeOcspResponder{
		certificate: GetCertificateFromString(cert),
		privateKey:  GetPrivateKeyRSAFromString(key),
		issuer:      rootCa,
	}
}

// GetOcspResponse returns a DER-encoded OCSP response for the given certificate.
// You pass it the certificate to get a response for, the expiration time of the response, and whether the certificate should be revoked.
// You can also pass it an ocsp.Response to use as a template for the response. This allows for customizing the response wanted.
func (f *FakeOcspResponder) GetOcspResponse(certificate *x509.Certificate, expiration time.Duration, isRevoked bool, resp ocsp.Response) []byte {
	template := resp
	template.Certificate = certificate
	status := ocsp.Good
	if isRevoked {
		status = ocsp.Revoked
	}

	template = ocsp.Response{
		Status:       status,
		SerialNumber: certificate.SerialNumber,
		NextUpdate:   time.Now().Add(expiration),
		Certificate:  certificate,
	}

	response, err := ocsp.CreateResponse(f.issuer, f.certificate, template, f.privateKey)
	Expect(err).NotTo(HaveOccurred())

	return response
}
