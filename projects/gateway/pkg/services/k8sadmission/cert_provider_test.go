package k8sadmission

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("certificateProvider", func() {

	var (
		certPath string
		keyPath  string
		err      error
		ctx      context.Context
		cancel   context.CancelFunc
	)
	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.TODO())
		certPath, keyPath, err = createCertificate()
		if err != nil {
			Fail(fmt.Sprintf("Failed to create certificate: %s", err))
		}
	})
	AfterEach(func() {
		cancel()
		if certPath != "" {
			os.Remove(certPath)
			certPath = ""
		}
		if keyPath != "" {
			os.Remove(keyPath)
			keyPath = ""
		}
	})

	It("reloads certificates upon changes", func() {
		logger := log.New(os.Stderr, "cert-provider-test", log.LstdFlags)
		provider, err := NewCertificateProvider(certPath, keyPath, logger, ctx, 1*time.Second)
		if err != nil {
			Fail(fmt.Sprintf("%s", err))
		}
		cert, err := provider.GetCertificateFunc()(nil)
		if err != nil {
			Fail(fmt.Sprintf("%s", err))
		}
		if cert == nil {
			Fail("certificate is nil")
		}
		err = createNewCertificate(certPath, keyPath)
		if err != nil {
			Fail(fmt.Sprintf("failed to update/replace certificate %s", err))
		}
		time.Sleep(3 * time.Second)
		cert2, err := provider.GetCertificateFunc()(nil)
		if err != nil {
			Fail(fmt.Sprintf("%s", err))
		}
		if cert == cert2 {
			Fail("certificate has not been reloaded")
		}
	})

	It("keeps same certificates if it has not changed", func() {
		logger := log.New(os.Stderr, "cert-provider-test", log.LstdFlags)
		provider, err := NewCertificateProvider(certPath, keyPath, logger, ctx, 1*time.Second)
		if err != nil {
			Fail(fmt.Sprintf("%s", err))
		}
		cert, err := provider.GetCertificateFunc()(nil)
		if err != nil {
			Fail(fmt.Sprintf("%s", err))
		}
		if cert == nil {
			Fail("certificate is nil")
		}
		time.Sleep(3 * time.Second)
		cert2, err := provider.GetCertificateFunc()(nil)
		if err != nil {
			Fail(fmt.Sprintf("%s", err))
		}
		if cert != cert2 {
			Fail("certificate has been reloaded altgough it has not changed")
		}
	})

	It("keeps old certificates on reload error", func() {
		logger := log.New(os.Stderr, "cert-provider-test", log.LstdFlags)
		provider, err := NewCertificateProvider(certPath, keyPath, logger, ctx, 1*time.Second)
		if err != nil {
			Fail(fmt.Sprintf("%s", err))
		}
		cert, err := provider.GetCertificateFunc()(nil)
		if err != nil {
			Fail(fmt.Sprintf("%s", err))
		}
		if cert == nil {
			Fail("certificate is nil")
		}
		err = os.Remove(certPath)
		if err != nil {
			Fail(fmt.Sprintf("failed to remove certificate file %s: %s", certPath, err))
		}
		time.Sleep(3 * time.Second)
		cert2, err := provider.GetCertificateFunc()(nil)
		if err != nil {
			Fail(fmt.Sprintf("%s", err))
		}
		if cert != cert2 {
			Fail("certificate has been reloaded altgough it has not changed")
		}
	})

})

func createCertificate() (certFile, keyFile string, err error) {
	err = nil
	var file *os.File
	file, err = os.CreateTemp("", "cert")
	if err != nil {
		return
	}
	certFile = file.Name()
	file, err = os.CreateTemp("", "key")
	if err != nil {
		return
	}
	keyFile = file.Name()
	err = createNewCertificate(certFile, keyFile)
	return
}

func createNewCertificate(certFile, keyFile string) error {
	priv, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return err
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Gloo"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour * 24 * 180),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"localhost", "*"},
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey(priv), priv)
	if err != nil {
		return err
	}
	out := &bytes.Buffer{}
	pem.Encode(out, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	err = os.WriteFile(certFile, out.Bytes(), 0644)
	if err != nil {
		return err
	}
	out.Reset()
	pemBlock, err := pemBlockForKey(priv)
	if err != nil {
		return err
	}
	pem.Encode(out, pemBlock)
	err = os.WriteFile(keyFile, out.Bytes(), 0644)
	if err != nil {
		return err
	}
	return nil
}

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

func pemBlockForKey(priv interface{}) (*pem.Block, error) {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}, nil
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return nil, err
		}
		return &pem.Block{Type: "EC PRIVATE KEY", Bytes: b}, nil
	default:
		return nil, nil
	}
}
