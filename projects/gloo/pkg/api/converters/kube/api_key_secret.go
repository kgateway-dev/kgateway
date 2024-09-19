package kubeconverters

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	"golang.org/x/net/http/httpguts"
	corev1 "k8s.io/api/core/v1"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
)

const (
	APIKeyDataKey                           = "api-key"
	APIKeySecretType      corev1.SecretType = "extauth.solo.io/apikey"
	GlooKindAnnotationKey                   = "resource_kind"
)

// Processes secrets with type "extauth.solo.io/apikey".
type APIKeySecretConverter struct{}

func ValidateAPIKeySecret(ctx context.Context, secret *corev1.Secret) *multierror.Error {
	var err *multierror.Error

	if secret == nil {
		return multierror.Append(err, fmt.Errorf("unexpected nil secret"))
	}

	if secret.Type == APIKeySecretType {
		_, hasAPIKey := secret.Data[APIKeyDataKey]
		if !hasAPIKey {
			return multierror.Append(err, fmt.Errorf("no api-key data field"))
		}

		// Copy remaining secret data to gloo secret metadata
		for key, value := range secret.Data {
			if key == APIKeyDataKey {
				continue
			}

			if !httpguts.ValidHeaderFieldValue(string(value)) {
				// v could be sensitive, only log k
				err = multierror.Append(err, fmt.Errorf("apikey had unresolvable value for header: %s", key))
			}

		}

		return err
	}

	err = multierror.Append(err, fmt.Errorf("unexpected secret type %v", secret.Type))
	return err

}

func (c *APIKeySecretConverter) FromKubeSecret(ctx context.Context, _ *kubesecret.ResourceClient, secret *corev1.Secret) (resources.Resource, error) {
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

			if !httpguts.ValidHeaderFieldName(key) {
				key = strings.TrimSpace(key)
				if !httpguts.ValidHeaderFieldName(key) {
					contextutils.LoggerFrom(ctx).Warnw("apikey had unresolvable header", zap.Any("header", key))
					//continue
				}
			}
			if !httpguts.ValidHeaderFieldValue(string(value)) {
				// v could be sensitive, only log k
				contextutils.LoggerFrom(ctx).Warnw("apikey had unresolvable headervalue", zap.Any("header", key), zap.String("value", string(value)))
				//return nil, eris.New("apikey had unresolvable headervalue")
				//continue
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

func (c *APIKeySecretConverter) ToKubeSecret(ctx context.Context, rc *kubesecret.ResourceClient, resource resources.Resource) (*corev1.Secret, error) {
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
		if key == APIKeyDataKey {
			continue
		}

		if !httpguts.ValidHeaderFieldName(key) {
			key = strings.TrimSpace(key)
			if !httpguts.ValidHeaderFieldName(key) {
				contextutils.LoggerFrom(ctx).Warnw("apikey had unresolvable header", zap.Any("header", key))
				//continue
			}
		}
		if !httpguts.ValidHeaderFieldValue(value) {
			// v could be sensitive, only log k
			contextutils.LoggerFrom(ctx).Warnw("apikey had unresolvable headervalue", zap.Any("header", key), zap.String("value", value))
			//return nil, eris.New("apikey had unresolvable headervalue")
			//continue
		}

		secretData[key] = value
	}

	kubeSecret := &corev1.Secret{
		ObjectMeta: kubeMeta,
		Type:       APIKeySecretType,
		StringData: secretData,
	}

	return kubeSecret, nil
}
