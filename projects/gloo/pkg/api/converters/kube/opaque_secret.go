package kubeconverters

import (
	"context"

	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"go.uber.org/zap"
	kubev1 "k8s.io/api/core/v1"
)

type OpaqueSecretConverter struct{}

var _ kubesecret.SecretConverter = &OpaqueSecretConverter{}

func (c *OpaqueSecretConverter) FromKubeSecret(ctx context.Context, rc *kubesecret.ResourceClient, secret *kubev1.Secret) (resources.Resource, error) {
	if secret == nil {
		contextutils.LoggerFrom(ctx).Warn("unexpected nil secret")
		return nil, nil
	}

	if secret.Type != kubev1.SecretTypeOpaque {
		return nil, nil
	}

	// let the ResourceClient try to unmarshal the opaque secret into one of the Gloo secret types
	resource, err := rc.FromKubeSecret(secret)
	if err != nil {
		// if that was unsuccessful, return nil to fall back to the next converter
		contextutils.LoggerFrom(ctx).Debugw("resource client could not convert secret", zap.Error(err))
		return nil, nil
	}

	return resource, nil
}

func (c *OpaqueSecretConverter) ToKubeSecret(_ context.Context, _ *kubesecret.ResourceClient, _ resources.Resource) (*kubev1.Secret, error) {
	// We don't need to do any special conversion from Gloo to k8s secrets. Just return nil and let next converter handle it
	return nil, nil
}
