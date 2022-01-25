package kubeconverters

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	"go.uber.org/zap"
	kubev1 "k8s.io/api/core/v1"
)

const (
	ClientSecretDataKey                   = "client-secret"
	OAuthSecretType     kubev1.SecretType = "extauth.solo.io/oauth"
)

// OAuthSecretConverter processes secrets with type "extauth.solo.io/oauth"
type OAuthSecretConverter struct{}

var _ kubesecret.SecretConverter = &OAuthSecretConverter{}

func (t *OAuthSecretConverter) FromKubeSecret(ctx context.Context, rc *kubesecret.ResourceClient, secret *kubev1.Secret) (resources.Resource, error) {
	if secret == nil {
		contextutils.LoggerFrom(ctx).Warn("unexpected nil secret")
		return nil, nil
	}

	if secret.Type == OAuthSecretType {
		clientSecret, hasClientSecret := secret.Data[ClientSecretDataKey]
		if !hasClientSecret {
			contextutils.LoggerFrom(ctx).Warnw("skipping OAuth secret with no client-secret data field",
				zap.String("name", secret.Name), zap.String("namespace", secret.Namespace))
			return nil, nil
		}

		oauthSecret := &extauthv1.OauthSecret{
			ClientSecret: string(clientSecret),
		}

		return &v1.Secret{
			Metadata: kubeutils.FromKubeMeta(secret.ObjectMeta, true),
			Kind: &v1.Secret_Oauth{
				Oauth: oauthSecret,
			},
		}, nil
	}

	// any unmatched secrets will be handled by subsequent converters
	return nil, nil
}

func (t *OAuthSecretConverter) ToKubeSecret(_ context.Context, rc *kubesecret.ResourceClient, resource resources.Resource) (*kubev1.Secret, error) {
	glooSecret, ok := resource.(*v1.Secret)
	if !ok {
		return nil, nil
	}
	oauthSecret, ok := glooSecret.GetKind().(*v1.Secret_Oauth)
	if !ok {
		return nil, nil
	}

	objectMeta := kubeutils.ToKubeMeta(glooSecret.GetMetadata())
	// If the secret we have in memory is a plain solo-kit secret (i.e. it was written to storage before
	// this converter was added), we take the chance to convert it to the new format.
	// As part of that we need to remove the `resource_kind: '*v1.Secret'` annotation.
	if len(objectMeta.Annotations) > 0 && objectMeta.Annotations[GlooKindAnnotationKey] == "*v1.Secret" {
		delete(objectMeta.Annotations, GlooKindAnnotationKey)
		if len(objectMeta.Annotations) == 0 {
			objectMeta.Annotations = nil
		}
	}

	return &kubev1.Secret{
		ObjectMeta: objectMeta,
		Type:       OAuthSecretType,
		Data: map[string][]byte{
			ClientSecretDataKey: []byte(oauthSecret.Oauth.GetClientSecret()),
		},
	}, nil
}
