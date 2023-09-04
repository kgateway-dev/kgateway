package kube_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/k8s-utils/certutils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	. "github.com/solo-io/gloo/jobs/pkg/kube"
)

var _ = Describe("Secret", func() {
	It("creates a tls secret from the provided certs", func() {
		data := []byte{1, 2, 3}
		kube := fake.NewSimpleClientset()
		secretCfg := TlsSecret{
			SecretName:         "mysecret",
			SecretNamespace:    "mynamespace",
			PrivateKeyFileName: "key.pem",
			CertFileName:       "ca.pem",
			CaBundleFileName:   "ca_bundle.pem",
			PrivateKey:         data,
			Cert:               data,
			CaBundle:           data,
		}

		_, err := CreateTlsSecret(context.TODO(), kube, secretCfg)
		Expect(err).NotTo(HaveOccurred())

		ctx, cancel := context.WithCancel(context.Background())
		defer func() { cancel() }()
		secret, err := kube.CoreV1().Secrets(secretCfg.SecretNamespace).Get(ctx, secretCfg.SecretName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(secret).To(Equal(&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mysecret",
				Namespace: "mynamespace",
			},
			Data: map[string][]byte{"key.pem": data, "ca.pem": data, "ca_bundle.pem": data},
			Type: "kubernetes.io/tls",
		}))
	})

	Context("Get existing valid TLS secret", func() {

		generateCaCertBytes := func(notBefore, notAfter time.Time) []byte {
			// CA cert
			serial := big.NewInt(1)
			subject := pkix.Name{
				CommonName: "test",
			}
			ca, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			tmpl := &x509.Certificate{
				IsCA:         true,
				NotBefore:    notBefore,
				NotAfter:     notAfter,
				PublicKey:    ca.Public(),
				SerialNumber: serial,
				Subject:      subject,
				DNSNames:     []string{"mysvcname.mysvcnamespace"},
			}
			tmpl.SubjectKeyId, _ = computeSKI(tmpl)

			// No SAN fields set
			rawCACert, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &ca.PublicKey, ca)
			caCert, _ := x509.ParseCertificate(rawCACert)

			certBytes := certutils.EncodeCertPEM(caCert)
			return append(certBytes, certBytes...)
		}

		It("doesn't error on non-existing secret", func() {
			kube := fake.NewSimpleClientset()

			secret, err := GetExistingValidTlsSecret(context.TODO(), kube, "mysecret", "mynamespace",
				"mysvcname", "mysvcnamespace")
			Expect(err).NotTo(HaveOccurred())
			Expect(secret).To(BeNil())
		})

		It("recognizes a tls secret that is still valid", func() {
			data := []byte{1, 2, 3}

			kube := fake.NewSimpleClientset()
			secretCfg := TlsSecret{
				SecretName:         "mysecret",
				SecretNamespace:    "mynamespace",
				PrivateKeyFileName: "tls.key",
				CertFileName:       "tls.crt",
				CaBundleFileName:   "ca.crt",
				PrivateKey:         data,
				Cert:               generateCaCertBytes(time.Now(), time.Now().Add(24*90*time.Hour)),
				CaBundle:           data,
			}

			_, err := CreateTlsSecret(context.TODO(), kube, secretCfg)
			Expect(err).NotTo(HaveOccurred())

			existing, err := GetExistingValidTlsSecret(context.TODO(), kube, "mysecret", "mynamespace",
				"mysvcname", "mysvcnamespace")
			Expect(err).NotTo(HaveOccurred())
			Expect(existing).NotTo(BeNil())
		})

		It("recognizes a tls secret that is invalid relative to now", func() {
			data := []byte{1, 2, 3}

			kube := fake.NewSimpleClientset()
			secretCfg := TlsSecret{
				SecretName:         "mysecret",
				SecretNamespace:    "mynamespace",
				PrivateKeyFileName: "tls.key",
				CertFileName:       "tls.crt",
				CaBundleFileName:   "ca.crt",
				PrivateKey:         data,
				Cert:               generateCaCertBytes(time.Now().Add(1*time.Minute), time.Now().Add(2*time.Minute)),
				CaBundle:           data,
			}

			_, err := CreateTlsSecret(context.TODO(), kube, secretCfg)
			Expect(err).NotTo(HaveOccurred())

			existing, err := GetExistingValidTlsSecret(context.TODO(), kube, "mysecret", "mynamespace",
				"mysvcname", "mysvcnamespace")
			Expect(err).NotTo(HaveOccurred())
			Expect(existing).To(BeNil())
		})

		It("recognizes a tls secret that is invalid relative to now, not first cert in chain", func() {
			data := []byte{1, 2, 3}

			goodCert := generateCaCertBytes(time.Now(), time.Now().Add(2*time.Minute))
			badCert := generateCaCertBytes(time.Now().Add(1*time.Minute), time.Now().Add(2*time.Minute))
			combinedCert := append(goodCert, badCert...)

			kube := fake.NewSimpleClientset()
			secretCfg := TlsSecret{
				SecretName:         "mysecret",
				SecretNamespace:    "mynamespace",
				PrivateKeyFileName: "tls.key",
				CertFileName:       "tls.crt",
				CaBundleFileName:   "ca.crt",
				PrivateKey:         data,
				Cert:               combinedCert,
				CaBundle:           data,
			}

			_, err := CreateTlsSecret(context.TODO(), kube, secretCfg)
			Expect(err).NotTo(HaveOccurred())

			existing, err := GetExistingValidTlsSecret(context.TODO(), kube, "mysecret", "mynamespace",
				"mysvcname", "mysvcnamespace")
			Expect(err).NotTo(HaveOccurred())
			Expect(existing).To(BeNil())
		})

		It("recognizes a tls secret that is invalid due to service mismatch", func() {
			data := []byte{1, 2, 3}

			kube := fake.NewSimpleClientset()
			secretCfg := TlsSecret{
				SecretName:         "mysecret",
				SecretNamespace:    "mynamespace",
				PrivateKeyFileName: "tls.key",
				CertFileName:       "tls.crt",
				CaBundleFileName:   "ca.crt",
				PrivateKey:         data,
				Cert:               generateCaCertBytes(time.Now(), time.Now().Add(1*time.Minute)),
				CaBundle:           data,
			}

			_, err := CreateTlsSecret(context.TODO(), kube, secretCfg)
			Expect(err).NotTo(HaveOccurred())

			existing, err := GetExistingValidTlsSecret(context.TODO(), kube, "mysecret", "mynamespace",
				"newservicename", "mysvcnamespace")
			Expect(err).NotTo(HaveOccurred())
			Expect(existing).To(BeNil())
		})

		It("recognizes a tls secret that is expiring soon", func() {
			data := []byte{1, 2, 3}

			kube := fake.NewSimpleClientset()
			secretCfg := TlsSecret{
				SecretName:         "mysecret",
				SecretNamespace:    "mynamespace",
				PrivateKeyFileName: "tls.key",
				CertFileName:       "tls.crt",
				CaBundleFileName:   "ca.crt",
				PrivateKey:         data,
				Cert:               generateCaCertBytes(time.Now(), time.Now().Add(2*time.Minute)),
				CaBundle:           data,
			}

			_, err := CreateTlsSecret(context.TODO(), kube, secretCfg)
			Expect(err).NotTo(HaveOccurred())

			existing, err := GetExistingValidTlsSecret(context.TODO(), kube, "mysecret", "mynamespace",
				"mysvcname", "mysvcnamespace")
			Expect(err).NotTo(HaveOccurred())
			Expect(existing).To(BeNil())
		})
	})
})

func computeSKI(template *x509.Certificate) ([]byte, error) {
	pub := template.PublicKey
	encodedPub, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}

	var subPKI subjectPublicKeyInfo
	_, err = asn1.Unmarshal(encodedPub, &subPKI)
	if err != nil {
		return nil, err
	}

	pubHash := sha1.Sum(subPKI.SubjectPublicKey.Bytes)
	return pubHash[:], nil
}

type subjectPublicKeyInfo struct {
	Algorithm        pkix.AlgorithmIdentifier
	SubjectPublicKey asn1.BitString
}
