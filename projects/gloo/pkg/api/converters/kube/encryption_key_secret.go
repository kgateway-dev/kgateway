package kubeconverters

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	kubev1 "k8s.io/api/core/v1"
)

const (
	EncryptionDataKey                         = "key"
	EncryptionKeySecretType kubev1.SecretType = "gloo.solo.io.EncryptionKeySecret"
)

// EncryptionSecretConverter processes secrets with type "gloo.solo.io.EncryptionKeySecret"
type EncryptionSecretConverter struct{}

var _ kubesecret.SecretConverter = &EncryptionSecretConverter{}

func (t *EncryptionSecretConverter) FromKubeSecret(ctx context.Context, rc *kubesecret.ResourceClient, secret *kubev1.Secret) (resources.Resource, error) {
	if secret == nil {
		contextutils.LoggerFrom(ctx).Warn("unexpected nil secret")
		return nil, nil
	}

	encryptionKey, hasEncryptionKey := secret.Data[EncryptionDataKey]
	if hasEncryptionKey && secret.Type == EncryptionKeySecretType {
		encryptionSecret := &v1.EncryptionKeySecret{
			Key: string(encryptionKey),
		}
		return &v1.Secret{
			Metadata: kubeutils.FromKubeMeta(secret.ObjectMeta, true),
			Kind: &v1.Secret_Encryption{
				Encryption: encryptionSecret,
			},
		}, nil
	}

	// any unmatched secrets will be handled by subsequent converters
	return nil, nil
}

func (t *EncryptionSecretConverter) ToKubeSecret(_ context.Context, rc *kubesecret.ResourceClient, resource resources.Resource) (*kubev1.Secret, error) {
	glooSecret, ok := resource.(*v1.Secret)
	if !ok {
		return nil, nil
	}
	encryptionSecret, ok := glooSecret.GetKind().(*v1.Secret_Encryption)
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
		Type:       EncryptionKeySecretType,
		Data: map[string][]byte{
			EncryptionDataKey: []byte(encryptionSecret.Encryption.GetKey()),
		},
	}, nil
}
