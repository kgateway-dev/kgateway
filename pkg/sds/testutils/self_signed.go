package testutils

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"
)

// MustSelfSignedPEM returns a valid matching RSA private key, leaf certificate,
// and CA certificate. For test simplicity, the CA is the same self-signed cert.
func MustSelfSignedPEM() (keyPEM, certPEM, caPEM []byte) {
	return mustSelfSignedPEMWithCN("sds-test", 1)
}

// MustSelfSignedPEMRotation1 returns a distinct self-signed key/cert/CA triple
// for rotation tests.
func MustSelfSignedPEMRotation1() (keyPEM, certPEM, caPEM []byte) {
	return mustSelfSignedPEMWithCN("sds-test-rot1", 2)
}

// MustSelfSignedPEMRotation2 returns another distinct self-signed key/cert/CA
// triple for rotation tests.
func MustSelfSignedPEMRotation2() (keyPEM, certPEM, caPEM []byte) {
	return mustSelfSignedPEMWithCN("sds-test-rot2", 3)
}

func mustSelfSignedPEMWithCN(commonName string, serial int64) (keyPEM, certPEM, caPEM []byte) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	caPEM = append([]byte(nil), certPEM...)

	return keyPEM, certPEM, caPEM
}
