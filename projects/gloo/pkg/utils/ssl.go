package utils

import (
	"fmt"

	envoyauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/config/grpc_credential/v2alpha"
	gogo_types "github.com/gogo/protobuf/types"

	"github.com/pkg/errors"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	MetadataPluginName = "envoy.grpc_credentials.file_based_metadata"
)

type SslConfigTranslator struct {
	secrets v1.SecretList
}

func NewSslConfigTranslator(secrets v1.SecretList) *SslConfigTranslator {
	return &SslConfigTranslator{
		secrets: secrets,
	}
}

func (s *SslConfigTranslator) ResolveUpstreamSslConfig(uc *v1.UpstreamSslConfig) (*envoyauth.UpstreamTlsContext, error) {
	common, err := s.ResolveCommonSslConfig(uc)
	if err != nil {
		return nil, err
	}
	return &envoyauth.UpstreamTlsContext{
		CommonTlsContext: common,
		Sni:              uc.Sni,
	}, nil
}
func (s *SslConfigTranslator) ResolveDownstreamSslConfig(dc *v1.SslConfig) (*envoyauth.DownstreamTlsContext, error) {
	common, err := s.ResolveCommonSslConfig(dc)
	if err != nil {
		return nil, err
	}
	var requireClientCert *gogo_types.BoolValue
	if common.ValidationContextType != nil {
		requireClientCert = &gogo_types.BoolValue{Value: true}
	}
	// show alpn for downstreams.
	// placing it on upstreams maybe problematic if they do not expose alpn.
	common.AlpnProtocols = []string{"h2", "http/1.1"}
	return &envoyauth.DownstreamTlsContext{
		CommonTlsContext:         common,
		RequireClientCertificate: requireClientCert,
	}, nil
}

type CertSource interface {
	GetSecretRef() *core.ResourceRef
	GetSslFiles() *v1.SSLFiles
	GetSds() *v1.SDSConfig
	GetVerifySubjectAltName() []string
}

func dataSourceGenerator(inlineDataSource bool) func(s string) *envoycore.DataSource {
	return func(s string) *envoycore.DataSource {
		if !inlineDataSource {
			return &envoycore.DataSource{
				Specifier: &envoycore.DataSource_Filename{
					Filename: s,
				},
			}
		}
		return &envoycore.DataSource{
			Specifier: &envoycore.DataSource_InlineString{
				InlineString: s,
			},
		}
	}
}

func buildSds(name string, sslSecrets *v1.SDSConfig) *envoyauth.SdsSecretConfig {
	config := &v2alpha.FileBasedMetadataConfig{
		SecretData: &envoycore.DataSource{
			Specifier: &envoycore.DataSource_Filename{
				Filename: sslSecrets.CallCredentials.FileCredentialSource.TokenFileName,
			},
		},
		HeaderKey: sslSecrets.CallCredentials.FileCredentialSource.Header,
	}
	any, _ := gogo_types.MarshalAny(config)

	gRPCConfig := &envoycore.GrpcService_GoogleGrpc{
		TargetUri:  sslSecrets.TargetUri,
		StatPrefix: "sds",
		ChannelCredentials: &envoycore.GrpcService_GoogleGrpc_ChannelCredentials{
			CredentialSpecifier: &envoycore.GrpcService_GoogleGrpc_ChannelCredentials_LocalCredentials{
				LocalCredentials: &envoycore.GrpcService_GoogleGrpc_GoogleLocalCredentials{},
			},
		},
		CallCredentials: []*envoycore.GrpcService_GoogleGrpc_CallCredentials{
			&envoycore.GrpcService_GoogleGrpc_CallCredentials{
				CredentialSpecifier: &envoycore.GrpcService_GoogleGrpc_CallCredentials_FromPlugin{
					FromPlugin: &envoycore.GrpcService_GoogleGrpc_CallCredentials_MetadataCredentialsFromPlugin{
						Name: MetadataPluginName,
						ConfigType: &envoycore.GrpcService_GoogleGrpc_CallCredentials_MetadataCredentialsFromPlugin_TypedConfig{
							TypedConfig: any},
					},
				},
			},
		},
	}

	return &envoyauth.SdsSecretConfig{
		Name: name,
		SdsConfig: &envoycore.ConfigSource{
			ConfigSourceSpecifier: &envoycore.ConfigSource_ApiConfigSource{
				ApiConfigSource: &envoycore.ApiConfigSource{
					ApiType: envoycore.ApiConfigSource_GRPC,
					GrpcServices: []*envoycore.GrpcService{
						{
							TargetSpecifier: &envoycore.GrpcService_GoogleGrpc_{
								GoogleGrpc: gRPCConfig,
							},
						},
					},
				},
			},
		},
	}
}

func (s *SslConfigTranslator) handleSds(sslSecrets *v1.SDSConfig, verifySan []string) (*envoyauth.CommonTlsContext, error) {
	if sslSecrets.CertificatesSecretName == "" && sslSecrets.ValidationContextName == "" {
		return nil, fmt.Errorf("at least one of certificates_secret_name or validation_context_name must be provided")
	}
	if len(verifySan) != 0 && sslSecrets.ValidationContextName == "" {
		return nil, fmt.Errorf("must provide validation context name if verifying SAN")
	}
	tlsContext := &envoyauth.CommonTlsContext{
		// default params
		TlsParams: &envoyauth.TlsParameters{},
	}

	if sslSecrets.CertificatesSecretName != "" {
		tlsContext.TlsCertificateSdsSecretConfigs = []*envoyauth.SdsSecretConfig{buildSds(sslSecrets.CertificatesSecretName, sslSecrets)}
	}

	if sslSecrets.ValidationContextName != "" {
		if len(verifySan) == 0 {
			tlsContext.ValidationContextType = &envoyauth.CommonTlsContext_ValidationContextSdsSecretConfig{
				ValidationContextSdsSecretConfig: buildSds(sslSecrets.ValidationContextName, sslSecrets),
			}
		} else {
			tlsContext.ValidationContextType = &envoyauth.CommonTlsContext_CombinedValidationContext{
				CombinedValidationContext: &envoyauth.CommonTlsContext_CombinedCertificateValidationContext{
					DefaultValidationContext:         &envoyauth.CertificateValidationContext{VerifySubjectAltName: verifySan},
					ValidationContextSdsSecretConfig: buildSds(sslSecrets.ValidationContextName, sslSecrets),
				},
			}
		}
	}

	return tlsContext, nil
}

func (s *SslConfigTranslator) ResolveCommonSslConfig(cs CertSource) (*envoyauth.CommonTlsContext, error) {
	var (
		certChain, privateKey, rootCa string
		// if using a Secret ref, we will inline the certs in the tls config
		inlineDataSource bool
	)

	if sslSecrets := cs.GetSecretRef(); sslSecrets != nil {
		var err error
		inlineDataSource = true
		ref := sslSecrets
		certChain, privateKey, rootCa, err = getSslSecrets(*ref, s.secrets)
		if err != nil {
			return nil, err
		}
	} else if sslSecrets := cs.GetSslFiles(); sslSecrets != nil {
		certChain, privateKey, rootCa = sslSecrets.TlsCert, sslSecrets.TlsKey, sslSecrets.RootCa
	} else if sslSecrets := cs.GetSds(); sslSecrets != nil {
		return s.handleSds(sslSecrets, cs.GetVerifySubjectAltName())
	} else {
		return nil, errors.New("no certificate information found")
	}

	dataSource := dataSourceGenerator(inlineDataSource)

	var certChainData, privateKeyData, rootCaData *envoycore.DataSource

	if certChain != "" {
		certChainData = dataSource(certChain)
	}
	if privateKey != "" {
		privateKeyData = dataSource(privateKey)
	}
	if rootCa != "" {
		rootCaData = dataSource(rootCa)
	}

	tlsContext := &envoyauth.CommonTlsContext{
		// default params
		TlsParams: &envoyauth.TlsParameters{},
	}

	if certChainData != nil && privateKeyData != nil {
		tlsContext.TlsCertificates = []*envoyauth.TlsCertificate{
			{
				CertificateChain: certChainData,
				PrivateKey:       privateKeyData,
			},
		}
	} else if certChainData != nil || privateKeyData != nil {
		return nil, errors.New("both or none of cert chain and private key must be provided")
	}

	sanList := cs.GetVerifySubjectAltName()

	if rootCaData != nil {
		validationCtx := &envoyauth.CommonTlsContext_ValidationContext{
			ValidationContext: &envoyauth.CertificateValidationContext{
				TrustedCa: rootCaData,
			},
		}
		if len(sanList) != 0 {
			validationCtx.ValidationContext.VerifySubjectAltName = sanList
		}
		tlsContext.ValidationContextType = validationCtx

	} else if len(sanList) != 0 {
		return nil, errors.New("a root_ca must be provided if verify_subject_alt_name is not empty")

	}

	return tlsContext, nil
}

func getSslSecrets(ref core.ResourceRef, secrets v1.SecretList) (string, string, string, error) {
	secret, err := secrets.Find(ref.Strings())
	if err != nil {
		return "", "", "", errors.Wrapf(err, "SSL secret not found")
	}

	sslSecret, ok := secret.Kind.(*v1.Secret_Tls)
	if !ok {
		return "", "", "", errors.Errorf("%v is not a TLS secret", secret.GetMetadata().Ref())
	}

	certChain := sslSecret.Tls.CertChain
	privateKey := sslSecret.Tls.PrivateKey
	rootCa := sslSecret.Tls.RootCa
	return certChain, privateKey, rootCa, nil
}
