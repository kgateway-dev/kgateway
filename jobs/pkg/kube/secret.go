package kube

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"time"

	errors "github.com/rotisserie/eris"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type TlsSecret struct {
	SecretName, SecretNamespace                        string
	PrivateKeyFileName, CertFileName, CaBundleFileName string
	PrivateKey, Cert, CaBundle                         []byte
}

func SecretExistsAndIsValidTlsSecret(ctx context.Context, kube kubernetes.Interface, secretCfg TlsSecret) (bool, error) {
	secretClient := kube.CoreV1().Secrets(secretCfg.SecretNamespace)

	existing, err := secretClient.Get(ctx, secretCfg.SecretName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			contextutils.LoggerFrom(ctx).Warnw("failed to retrieve existing secret",
				zap.String("secretName", secretCfg.SecretName),
				zap.String("secretNamespace", secretCfg.SecretNamespace))
			// necessary to return no errors in this case so we don't short circuit certgen on the first run
			return false, nil
		}
		return false, errors.Wrapf(err, "failed to retrieve existing secret")
	}

	if existing.Type != v1.SecretTypeTLS {
		return false, errors.Errorf("unexpected secret type, expected %s and got %s", v1.SecretTypeTLS, existing.Type)
	}

	certPemBytes := existing.Data[v1.TLSCertKey]
	now := time.Now().UTC()

	rest := certPemBytes
	for len(rest) > 0 {
		var decoded *pem.Block
		decoded, rest = pem.Decode(rest)
		if decoded == nil {
			return false, errors.New("no PEM data found")
		}
		cert, err := x509.ParseCertificate(decoded.Bytes)
		if err != nil {
			return false, errors.Wrapf(err, "failed to decode pem encoded ca cert")
		}

		if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
			return false, nil
		}
	}

	// cert is still valid!
	return true, nil
}

func CreateTlsSecret(ctx context.Context, kube kubernetes.Interface, secretCfg TlsSecret) error {
	secret := makeTlsSecret(secretCfg)

	secretClient := kube.CoreV1().Secrets(secret.Namespace)

	contextutils.LoggerFrom(ctx).Infow("creating TLS secret", zap.String("secret", secret.Name))

	if _, err := secretClient.Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		if apierrors.IsAlreadyExists(err) {
			contextutils.LoggerFrom(ctx).Infow("existing TLS secret found, attempting to update",
				zap.String("secretName", secret.Name),
				zap.String("secretNamespace", secret.Namespace))

			existing, err := secretClient.Get(ctx, secret.Name, metav1.GetOptions{})
			if err != nil {
				return errors.Wrapf(err, "failed to retrieve existing secret after receiving AlreadyExists error on Create")
			}

			secret.ResourceVersion = existing.ResourceVersion

			if _, err := secretClient.Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
				return errors.Wrapf(err, "failed updating existing secret")
			}
			return nil
		}
		return errors.Wrapf(err, "failed creating secret")
	}

	return nil
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
