package kube

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type TlsSecret struct {
	SecretName, SecretNamespace string
	PrivateKeyKey, CaCertKey    string
	PrivateKey, CaCert          []byte
}

func CreateTlsSecret(ctx context.Context, kube kubernetes.Interface, secretCfg TlsSecret) error {
	secret := makeTlsSecret(secretCfg)

	secretClient := kube.CoreV1().Secrets(secret.Namespace)

	contextutils.LoggerFrom(ctx).Infow("creating TLS secret", zap.String("secret", secret.Name))

	if _, err := secretClient.Create(secret); err != nil {
		if apierrors.IsAlreadyExists(err) {
			contextutils.LoggerFrom(ctx).Infow("existing TLS secret found, attempting to update", zap.String("secret", secret.Name))

			existing, err := secretClient.Get(secret.Name, metav1.GetOptions{})
			if err != nil {
				return errors.Wrapf(err, "failed to retrieve existing secret after receiving AlreadyExists error on Create")
			}
			secret.ResourceVersion = existing.ResourceVersion

			if _, err := secretClient.Update(secret); err != nil {
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
			args.PrivateKeyKey: args.PrivateKey,
			args.CaCertKey:     args.CaCert,
		},
	}
}
