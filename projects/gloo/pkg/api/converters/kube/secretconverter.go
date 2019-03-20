package kubeconverters

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"

	kubev1 "k8s.io/api/core/v1"
)

const (
	annotationKey   = "solo.io/secret-converter"
	annotationValue = "kube-tls"
)

type TLSSecretConverter struct{}

func (t *TLSSecretConverter) FromKubeSecret(ctx context.Context, rc *kubesecret.ResourceClient, secret *kubev1.Secret) (resources.Resource, error) {

	if secret.Type == kubev1.SecretTypeTLS {
		glooSecret := &v1.Secret{

			Kind: &v1.Secret_Tls{
				Tls: &v1.TlsSecret{
					PrivateKey: string(secret.Data[kubev1.TLSPrivateKeyKey]),
					CertChain:  string(secret.Data[kubev1.TLSCertKey]),
				},
			},
			Metadata: kubeutils.FromKubeMeta(secret.ObjectMeta),
		}
		if glooSecret.Metadata.Annotations == nil {
			glooSecret.Metadata.Annotations = make(map[string]string)
		}
		glooSecret.Metadata.Annotations[annotationKey] = annotationValue
		return glooSecret, nil
	}

	// this is temporary until https://github.com/solo-io/solo-kit/pull/110 is merged
	// then we can just return nil all the time
	if rc == nil {
		return nil, nil
	}

	return rc.FromKubeSecret(secret)
}

func (t *TLSSecretConverter) ToKubeSecret(ctx context.Context, rc *kubesecret.ResourceClient, resource resources.Resource) (*kubev1.Secret, error) {

	if glooSecret, ok := resource.(*v1.Secret); ok {
		if tlsGlooSecret, ok := glooSecret.Kind.(*v1.Secret_Tls); ok {
			if glooSecret.Metadata.Annotations != nil {
				if glooSecret.Metadata.Annotations[annotationKey] == annotationValue {
					objectMeta := kubeutils.ToKubeMeta(glooSecret.Metadata)
					delete(objectMeta.Annotations, annotationKey)
					if len(objectMeta.Annotations) == 0 {
						objectMeta.Annotations = nil
					}
					kubeSecret := &kubev1.Secret{
						ObjectMeta: objectMeta,
						Type:       kubev1.SecretTypeTLS,
						Data: map[string][]byte{
							kubev1.TLSPrivateKeyKey: []byte(tlsGlooSecret.Tls.PrivateKey),
							kubev1.TLSCertKey:       []byte(tlsGlooSecret.Tls.CertChain),
						},
					}
					return kubeSecret, nil
				}
			}
		}
	}

	// this is temporary until https://github.com/solo-io/solo-kit/pull/110 is merged
	// then we can just return nil all the time
	if rc == nil {
		return nil, nil
	}

	return rc.ToKubeSecret(ctx, resource)

}
