package kubeconverters

import (
	"context"

	skcore "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	kubev1 "k8s.io/api/core/v1"
)

type HeaderSecretConverter struct{}

var _ kubesecret.SecretConverter = &HeaderSecretConverter{}

const HeaderSecretType = "gloo.solo.io/header"

func (t *HeaderSecretConverter) FromKubeSecret(ctx context.Context, _ *kubesecret.ResourceClient, secret *kubev1.Secret) (resources.Resource, error) {
	if secret == nil {
		contextutils.LoggerFrom(ctx).Warn("unexpected nil secret")
		return nil, nil
	}

	if secret.Type == HeaderSecretType || secret.Type == kubev1.SecretTypeOpaque {
		if len(secret.Data) == 0 {
			if secret.Type == HeaderSecretType {
				// only log this warning for header secrets (we don't want it to show for all opaque secrets)
				contextutils.LoggerFrom(ctx).Warnw("skipping header secret with no headers",
					zap.String("name", secret.Name), zap.String("namespace", secret.Namespace))
			}
			return nil, nil
		}

		headers := make(map[string]string)
		for name, byteValue := range secret.Data {
			headers[name] = string(byteValue)
		}

		skSecret := &v1.Secret{
			Metadata: &skcore.Metadata{
				Name:        secret.Name,
				Namespace:   secret.Namespace,
				Labels:      secret.Labels,
				Annotations: secret.Annotations,
			},
			Kind: &v1.Secret_Header{
				Header: &v1.HeaderSecret{
					Headers: headers,
				},
			},
		}

		return skSecret, nil
	}
	// any unmatched secrets will be handled by subsequent converters
	return nil, nil
}

func (t *HeaderSecretConverter) ToKubeSecret(_ context.Context, _ *kubesecret.ResourceClient, resource resources.Resource) (*kubev1.Secret, error) {
	glooSecret, ok := resource.(*v1.Secret)
	if !ok {
		return nil, nil
	}
	headerGlooSecret, ok := glooSecret.GetKind().(*v1.Secret_Header)
	if !ok {
		return nil, nil
	}

	kubeMeta := kubeutils.ToKubeMeta(glooSecret.GetMetadata())

	kubeSecret := &kubev1.Secret{
		ObjectMeta: kubeMeta,
		Type:       HeaderSecretType,
		StringData: headerGlooSecret.Header.GetHeaders(),
	}

	return kubeSecret, nil
}
