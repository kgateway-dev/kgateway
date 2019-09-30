package kubeconverters

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/protoutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	skcore "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	kubev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

const (
	annotationKey   = "solo.io/secret-converter"
	annotationValue = "kube-tls"
)

type SecretConverterChain struct {
	converters []kubesecret.SecretConverter
}

var _ kubesecret.SecretConverter = &SecretConverterChain{}

func NewSecretConverterChain(converters ...kubesecret.SecretConverter) *SecretConverterChain {
	return &SecretConverterChain{converters: converters}
}

func (t *SecretConverterChain) FromKubeSecret(ctx context.Context, rc *kubesecret.ResourceClient, secret *kubev1.Secret) (resources.Resource, error) {
	for _, converter := range t.converters {
		resource, err := converter.FromKubeSecret(ctx, rc, secret)
		if err != nil {
			return nil, err
		}
		if resource != nil {
			return resource, nil
		}
	}
	// any unmatched secrets will be handled by subsequent converters
	return nil, nil
}

func (t *SecretConverterChain) ToKubeSecret(ctx context.Context, rc *kubesecret.ResourceClient, resource resources.Resource) (*kubev1.Secret, error) {
	for _, converter := range t.converters {
		kubeSecret, err := converter.ToKubeSecret(ctx, rc, resource)
		if err != nil {
			return nil, err
		}
		if kubeSecret != nil {
			return kubeSecret, nil
		}
	}
	// any unmatched secrets will be handled by subsequent converters
	return nil, nil
}

type TLSSecretConverter struct{}

var _ kubesecret.SecretConverter = &TLSSecretConverter{}

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
	// any unmatched secrets will be handled by subsequent converters
	return nil, nil
}

func (t *TLSSecretConverter) ToKubeSecret(_ context.Context, _ *kubesecret.ResourceClient, resource resources.Resource) (*kubev1.Secret, error) {
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
	// any unmatched secrets will be handled by subsequent converters
	return nil, nil
}

// The purpose of this implementation of the SecretConverter interface is to provide a way for the user to specify AWS
// secrets without having to use an annotation to identify the secret as a AWS secret. Instead of an annotation, this
// converter looks for the two required fields.
type AwsSecretConverter struct{}

var _ kubesecret.SecretConverter = &AwsSecretConverter{}

const (
	AwsAccessKeyName = "aws_access_key_id"
	AwsSecretKeyName = "aws_secret_access_key"
)

func (t *AwsSecretConverter) FromKubeSecret(ctx context.Context, rc *kubesecret.ResourceClient, secret *kubev1.Secret) (resources.Resource, error) {
	accessKey, hasAccessKey := secret.Data[AwsAccessKeyName]
	secretKey, hasSecretKey := secret.Data[AwsSecretKeyName]
	if hasAccessKey && hasSecretKey {
		return &v1.Secret{
			Metadata: skcore.Metadata{
				Name:        secret.Name,
				Namespace:   secret.Namespace,
				Cluster:     secret.ClusterName,
				Labels:      secret.Labels,
				Annotations: secret.Annotations,
			},
			Kind: &v1.Secret_Aws{
				Aws: &v1.AwsSecret{
					AccessKey: string(accessKey),
					SecretKey: string(secretKey),
				},
			},
		}, nil
	}
	// any unmatched secrets will be handled by subsequent converters
	return nil, nil
}

func (t *AwsSecretConverter) ToKubeSecret(ctx context.Context, rc *kubesecret.ResourceClient, resource resources.Resource) (*kubev1.Secret, error) {
	glooSecret, ok := resource.(*v1.Secret)
	if !ok {
		return nil, nil
	}
	awsGlooSecret, ok := glooSecret.Kind.(*v1.Secret_Aws)
	if !ok {
		return nil, nil
	}
	objectMeta := kubeutils.ToKubeMeta(glooSecret.Metadata)
	delete(objectMeta.Annotations, annotationKey)
	if len(objectMeta.Annotations) == 0 {
		objectMeta.Annotations = nil
	}
	awsBytes, err := protoutils.MarshalBytes(awsGlooSecret.Aws)
	if err != nil {
		return nil, err
	}
	awsBytes, err = yaml.JSONToYAML(awsBytes)
	if err != nil {
		return nil, err
	}

	kubeSecret := &kubev1.Secret{
		ObjectMeta: objectMeta,
		Type:       kubev1.SecretTypeOpaque,
		Data: map[string][]byte{
			AwsAccessKeyName: []byte(awsGlooSecret.Aws.AccessKey),
			AwsSecretKeyName: []byte(awsGlooSecret.Aws.SecretKey),
		},
	}
	return kubeSecret, nil
}
