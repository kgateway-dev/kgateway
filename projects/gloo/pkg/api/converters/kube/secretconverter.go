package kubeconverters

import (
	"context"
	"errors"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"

	kubev1 "k8s.io/api/core/v1"
)

type TLSSecretConverter struct{}

func (t *TLSSecretConverter) FromKubeSecret(ctx context.Context, rc *kubesecret.ResourceClient, secret *kubev1.Secret) (resources.Resource, error) {

	if secret.Type == kubev1.SecretTypeTLS {
		return &v1.Secret{
			Kind: &v1.Secret_Tls{
				Tls: &v1.TlsSecret{
					PrivateKey: string(secret.Data[kubev1.TLSPrivateKeyKey]),
					CertChain:  string(secret.Data[kubev1.TLSCertKey]),
				},
			},
			Metadata: kubeutils.FromKubeMeta(secret.ObjectMeta),
		}, nil
	}

	return rc.FromKubeSecret(secret)
}

func (t *TLSSecretConverter) ToKubeSecret(ctx context.Context, rc *kubesecret.ResourceClient, resource resources.Resource) (*kubev1.Secret, error) {

	return nil, errors.New("cannot convert gloo secret to kube secret - not implemented")
}
