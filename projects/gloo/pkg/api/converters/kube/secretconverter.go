package kubeconverters

import (
	"context"

	"github.com/solo-io/gloo/pkg/utils/protoutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	skcore "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	kubev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

const OCSPStapleKey = "tls.ocsp-staple"

var GlooSecretConverterChain = NewSecretConverterChain(
	new(TLSSecretConverter),
	new(AwsSecretConverter),
	new(APIKeySecretConverter),
	new(OAuthSecretConverter),
	new(AccountCredentialsSecretConverter),
	new(OpaqueSecretConverter),
	// added the encryption secret primarily for usersession cookie encryption. This enables us to
	// create and update the key used for encryption.
	new(EncryptionSecretConverter),
	// the header converter needs to run last because it has a fall-back to convert any opaque k8s secret with
	// non-empty data into a gloo header secret
	new(HeaderSecretConverter),
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

func (t *TLSSecretConverter) FromKubeSecret(_ context.Context, _ *kubesecret.ResourceClient, secret *kubev1.Secret) (resources.Resource, error) {
	if secret.Type == kubev1.SecretTypeTLS {
		glooSecret := &v1.Secret{
			Kind: &v1.Secret_Tls{
				Tls: &v1.TlsSecret{
					PrivateKey: string(secret.Data[kubev1.TLSPrivateKeyKey]),
					CertChain:  string(secret.Data[kubev1.TLSCertKey]),
					RootCa:     string(secret.Data[kubev1.ServiceAccountRootCAKey]),
					OcspStaple: secret.Data[OCSPStapleKey],
				},
			},
			Metadata: kubeutils.FromKubeMeta(secret.ObjectMeta, true),
		}
		return glooSecret, nil
	}
	// any unmatched secrets will be handled by subsequent converters
	return nil, nil
}

func (t *TLSSecretConverter) ToKubeSecret(_ context.Context, _ *kubesecret.ResourceClient, resource resources.Resource) (*kubev1.Secret, error) {
	if glooSecret, ok := resource.(*v1.Secret); ok {
		if tlsGlooSecret, ok := glooSecret.GetKind().(*v1.Secret_Tls); ok {
			objectMeta := kubeutils.ToKubeMeta(glooSecret.GetMetadata())
			if len(objectMeta.Annotations) == 0 {
				objectMeta.Annotations = nil
			}
			kubeSecret := &kubev1.Secret{
				ObjectMeta: objectMeta,
				Type:       kubev1.SecretTypeTLS,
				Data: map[string][]byte{
					kubev1.TLSPrivateKeyKey: []byte(tlsGlooSecret.Tls.GetPrivateKey()),
					kubev1.TLSCertKey:       []byte(tlsGlooSecret.Tls.GetCertChain()),
				},
			}

			if tlsGlooSecret.Tls.GetRootCa() != "" {
				kubeSecret.Data[kubev1.ServiceAccountRootCAKey] = []byte(tlsGlooSecret.Tls.GetRootCa())
			}

			if tlsGlooSecret.Tls.GetOcspStaple() != nil {
				kubeSecret.Data[OCSPStapleKey] = tlsGlooSecret.Tls.GetOcspStaple()
			}

			return kubeSecret, nil
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
	AwsAccessKeyName    = "aws_access_key_id"
	AwsSecretKeyName    = "aws_secret_access_key"
	AwsSessionTokenName = "aws_session_token"
)

func (t *AwsSecretConverter) FromKubeSecret(_ context.Context, _ *kubesecret.ResourceClient, secret *kubev1.Secret) (resources.Resource, error) {
	accessKey, hasAccessKey := secret.Data[AwsAccessKeyName]
	secretKey, hasSecretKey := secret.Data[AwsSecretKeyName]
	sessionToken, hasSessionToken := secret.Data[AwsSessionTokenName]
	if hasAccessKey && hasSecretKey {
		skSecret := &v1.Secret{
			Metadata: &skcore.Metadata{
				Name:        secret.Name,
				Namespace:   secret.Namespace,
				Labels:      secret.Labels,
				Annotations: secret.Annotations,
			},
			Kind: &v1.Secret_Aws{
				Aws: &v1.AwsSecret{
					AccessKey: string(accessKey),
					SecretKey: string(secretKey),
				},
			},
		}

		if hasSessionToken {
			skSecret.GetAws().SessionToken = string(sessionToken)
		}

		return skSecret, nil
	}
	// any unmatched secrets will be handled by subsequent converters
	return nil, nil
}

func (t *AwsSecretConverter) ToKubeSecret(_ context.Context, _ *kubesecret.ResourceClient, resource resources.Resource) (*kubev1.Secret, error) {
	glooSecret, ok := resource.(*v1.Secret)
	if !ok {
		return nil, nil
	}
	awsGlooSecret, ok := glooSecret.GetKind().(*v1.Secret_Aws)
	if !ok {
		return nil, nil
	}
	objectMeta := kubeutils.ToKubeMeta(glooSecret.GetMetadata())
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
			AwsAccessKeyName: []byte(awsGlooSecret.Aws.GetAccessKey()),
			AwsSecretKeyName: []byte(awsGlooSecret.Aws.GetSecretKey()),
		},
	}

	if sessionToken := awsGlooSecret.Aws.GetSessionToken(); sessionToken != "" {
		kubeSecret.Data[AwsSessionTokenName] = []byte(sessionToken)
	}
	return kubeSecret, nil
}
