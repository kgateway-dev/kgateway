package kube

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"time"

	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/jobs/pkg/certgen"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type TlsSecret struct {
	SecretName, SecretNamespace                        string
	PrivateKeyFileName, CertFileName, CaBundleFileName string
	PrivateKey, Cert, CaBundle                         []byte
}

// If there is a currently valid TLS secret with the given name and namespace, that is valid for the given
// service name/namespace, then return it. Otherwise return nil.
func GetExistingValidTlsSecret(ctx context.Context, kube kubernetes.Interface, secretName string, secretNamespace string,
	svcName string, svcNamespace string) (*v1.Secret, error) {
	secretClient := kube.CoreV1().Secrets(secretNamespace)

	existing, err := secretClient.Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			contextutils.LoggerFrom(ctx).Warnw("failed to retrieve existing secret",
				zap.String("secretName", secretName),
				zap.String("secretNamespace", secretNamespace))
			// necessary to return no errors in this case so we don't short circuit certgen on the first run
			return nil, nil
		}
		return nil, errors.Wrapf(err, "failed to retrieve existing secret")
	}

	if existing.Type != v1.SecretTypeTLS {
		return nil, errors.Errorf("unexpected secret type, expected %s and got %s", v1.SecretTypeTLS, existing.Type)
	}

	certPemBytes := existing.Data[v1.TLSCertKey]
	now := time.Now().UTC()

	rest := certPemBytes
	matchesSvc := false
	for len(rest) > 0 {
		var decoded *pem.Block
		decoded, rest = pem.Decode(rest)
		if decoded == nil {
			return nil, errors.New("no PEM data found")
		}
		cert, err := x509.ParseCertificate(decoded.Bytes)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode pem encoded ca cert")
		}

		// Create new certificate if old one is expiring soon (two months)
		if now.Before(cert.NotBefore) || now.After(cert.NotAfter.AddDate(0, -2, 0)) {
			return nil, nil
		}

		// check if the cert is valid for this service
		if !matchesSvc && certgen.ValidForService(cert.DNSNames, svcName, svcNamespace) {
			matchesSvc = true
		}
	}

	if !matchesSvc {
		return nil, nil
	}
	// cert is valid!
	return existing, nil
}

// Returns the created or updated secret
func CreateTlsSecret(ctx context.Context, kube kubernetes.Interface, secretCfg TlsSecret) (*v1.Secret, error) {
	secret := makeTlsSecret(secretCfg)

	secretClient := kube.CoreV1().Secrets(secret.Namespace)

	contextutils.LoggerFrom(ctx).Infow("creating TLS secret", zap.String("secret", secret.Name))

	createdSecret, err := secretClient.Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			contextutils.LoggerFrom(ctx).Infow("existing TLS secret found, attempting to update",
				zap.String("secretName", secret.Name),
				zap.String("secretNamespace", secret.Namespace))

			existing, err := secretClient.Get(ctx, secret.Name, metav1.GetOptions{})
			if err != nil {
				return nil, errors.Wrapf(err, "failed to retrieve existing secret after receiving AlreadyExists error on Create")
			}

			secret.ResourceVersion = existing.ResourceVersion

			updatedSecret, err := secretClient.Update(ctx, secret, metav1.UpdateOptions{})
			if err != nil {
				return nil, errors.Wrapf(err, "failed updating existing secret")
			}
			return updatedSecret, nil
		}
		return nil, errors.Wrapf(err, "failed creating secret")
	}

	return createdSecret, nil
}

func makeTlsSecret(args TlsSecret) *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      args.SecretName,
			Namespace: args.SecretNamespace,
		},
		Type: v1.SecretTypeTLS,
		Data: map[string][]byte{
			args.PrivateKeyFileName: args.PrivateKey,
			args.CertFileName:       args.Cert,
			args.CaBundleFileName:   args.CaBundle,
		},
	}
}
