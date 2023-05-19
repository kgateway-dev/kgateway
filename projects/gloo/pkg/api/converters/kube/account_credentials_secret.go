package kubeconverters

import (
	"context"

	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	skcore "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	kubev1 "k8s.io/api/core/v1"
)

const (
	AccountCredentialsSecretType kubev1.SecretType = "extauth.solo.io/accountcredentials"
	UsernameDataKey                                = "username"
	PasswordDataKey                                = "password"
)

type AccountCredentialsSecretConverter struct{}

func (t *AccountCredentialsSecretConverter) FromKubeSecret(_ context.Context, _ *kubesecret.ResourceClient, secret *kubev1.Secret) (resources.Resource, error) {
	if secret.Type == AccountCredentialsSecretType {
		username, ok := secret.Data[UsernameDataKey]
		if !ok {
			return nil, nil
		}
		password, ok := secret.Data[PasswordDataKey]
		if !ok {
			return nil, nil
		}
		skSecret := &v1.Secret{
			Metadata: &skcore.Metadata{
				Name:        secret.Name,
				Namespace:   secret.Namespace,
				Labels:      secret.Labels,
				Annotations: secret.Annotations,
			},
			Kind: &v1.Secret_Credentials{
				Credentials: &v1.AccountCredentialsSecret{
					Username: string(username),
					Password: string(password),
				},
			},
		}

		return skSecret, nil
	}
	// any unmatched secrets will be handled by subsequent converters
	return nil, nil
}
func (t *AccountCredentialsSecretConverter) ToKubeSecret(_ context.Context, _ *kubesecret.ResourceClient, resource resources.Resource) (*kubev1.Secret, error) {
	glooSecret, ok := resource.(*v1.Secret)
	if !ok {
		return nil, nil
	}
	credentialsSecret, ok := glooSecret.GetKind().(*v1.Secret_Credentials)
	if !ok {
		return nil, nil
	}

	kubeMeta := kubeutils.ToKubeMeta(glooSecret.GetMetadata())
	storedData := make(map[string]string)
	storedData[UsernameDataKey] = credentialsSecret.Credentials.GetUsername()
	storedData[PasswordDataKey] = credentialsSecret.Credentials.GetPassword()
	kubeSecret := &kubev1.Secret{
		ObjectMeta: kubeMeta,
		Type:       AccountCredentialsSecretType,
		StringData: storedData,
	}

	return kubeSecret, nil
}
