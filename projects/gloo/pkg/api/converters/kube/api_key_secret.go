package kubeconverters

import (
	"context"

	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	kubev1 "k8s.io/api/core/v1"
)

const (
	APIKeyDataKey                           = "api-key"
	APIKeySecretType      kubev1.SecretType = "extauth.solo.io/apikey"
	GlooKindAnnotationKey                   = "resource_kind"
)

// Processes secrets with type "extauth.solo.io/apikey".
type APIKeySecretConverter struct{}

func (c *APIKeySecretConverter) FromKubeSecret(ctx context.Context, _ *kubesecret.ResourceClient, secret *kubev1.Secret) (resources.Resource, error) {
	if secret == nil {
		contextutils.LoggerFrom(ctx).Warn("unexpected nil secret")
		return nil, nil
	}

	if secret.Type == APIKeySecretType {
		apiKey, hasAPIKey := secret.Data[APIKeyDataKey]
		if !hasAPIKey {
			contextutils.LoggerFrom(ctx).Warnw("skipping API key secret with no api-key data field",
				zap.String("name", secret.Name), zap.String("namespace", secret.Namespace))
			return nil, nil
		}

		apiKeySecret := &extauthv1.ApiKey{
			ApiKey: string(apiKey),
		}

		if len(secret.Data) > 1 {
			apiKeySecret.Metadata = map[string]string{}
		}

		// Copy remaining secret data to gloo secret metadata
		for key, value := range secret.Data {
			if key == APIKeyDataKey {
				continue
			}
			apiKeySecret.GetMetadata()[key] = string(value)
		}

		glooSecret := &v1.Secret{
			Metadata: kubeutils.FromKubeMeta(secret.ObjectMeta, true),
			Kind: &v1.Secret_ApiKey{
				ApiKey: apiKeySecret,
			},
		}

		return glooSecret, nil
	}

	return nil, nil
}

func (c *APIKeySecretConverter) ToKubeSecret(_ context.Context, rc *kubesecret.ResourceClient, resource resources.Resource) (*kubev1.Secret, error) {
	glooSecret, ok := resource.(*v1.Secret)
	if !ok {
		return nil, nil
	}
	apiKeyGlooSecret, ok := glooSecret.GetKind().(*v1.Secret_ApiKey)
	if !ok {
		return nil, nil
	}

	kubeMeta := kubeutils.ToKubeMeta(glooSecret.GetMetadata())

	// If the secret we have in memory is a plain solo-kit secret (i.e. it was written to storage before
	// this converter was added), we take the chance to convert it to the new format.
	// As part of that we need to remove the `resource_kind: '*v1.Secret'` annotation.
	if len(kubeMeta.Annotations) > 0 && kubeMeta.Annotations[GlooKindAnnotationKey] == rc.Kind() {
		delete(kubeMeta.Annotations, GlooKindAnnotationKey)
	}

	secretData := map[string]string{
		APIKeyDataKey: apiKeyGlooSecret.ApiKey.GetApiKey(),
	}

	for key, value := range apiKeyGlooSecret.ApiKey.GetMetadata() {
		secretData[key] = value
	}

	kubeSecret := &kubev1.Secret{
		ObjectMeta: kubeMeta,
		Type:       APIKeySecretType,
		StringData: secretData,
	}

	return kubeSecret, nil
}
