package utils

import (
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoygrpccredential "github.com/envoyproxy/go-control-plane/envoy/config/grpc_credential/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoymatcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

//go:generate mockgen -destination mocks/mock_ssl.go github.com/solo-io/gloo/projects/gloo/pkg/utils SslConfigTranslator

const (
	MetadataPluginName    = "envoy.grpc_credentials.file_based_metadata"
	defaultSdsClusterName = "gateway_proxy_sds"
)

var (
	TlsVersionNotFoundError = func(v v1.SslParameters_ProtocolVersion) error {
		return eris.Errorf("tls version %v not found", v)
	}

	SslSecretNotFoundError = func(err error) error {
		return eris.Wrapf(err, "SSL secret not found")
	}

	NotTlsSecretError = func(ref *core.ResourceRef) error {
		return eris.Errorf("%v is not a TLS secret", ref)
	}

	NoCertificateFoundError = eris.New("no certificate information found")

	MissingValidationContextError = eris.Errorf("must provide validation context name if verifying SAN")

	RootCaMustBeProvidedError = eris.Errorf("a root_ca must be provided if verify_subject_alt_name is not empty")
)

type SslConfigTranslator interface {
	ResolveUpstreamSslConfig(secrets v1.SecretList, uc *v1.UpstreamSslConfig) (*envoyauth.UpstreamTlsContext, error)
	ResolveDownstreamSslConfig(secrets v1.SecretList, dc *v1.SslConfig) (*envoyauth.DownstreamTlsContext, error)
	ResolveCommonSslConfig(cs CertSource, secrets v1.SecretList, mustHaveCert bool) (*envoyauth.CommonTlsContext, error)
	ResolveSslParamsConfig(params *v1.SslParameters) (*envoyauth.TlsParameters, error)
}

type sslConfigTranslator struct {
}

func NewSslConfigTranslator() *sslConfigTranslator {
	return &sslConfigTranslator{}
}

func (s *sslConfigTranslator) ResolveUpstreamSslConfig(secrets v1.SecretList, uc *v1.UpstreamSslConfig) (*envoyauth.UpstreamTlsContext, error) {
	common, err := s.ResolveCommonSslConfig(uc, secrets, false)
	if err != nil {
		return nil, err
	}
	return &envoyauth.UpstreamTlsContext{
		CommonTlsContext: common,
		Sni:              uc.Sni,
	}, nil
}
func (s *sslConfigTranslator) ResolveDownstreamSslConfig(secrets v1.SecretList, dc *v1.SslConfig) (*envoyauth.DownstreamTlsContext, error) {
	common, err := s.ResolveCommonSslConfig(dc, secrets, true)
	if err != nil {
		return nil, err
	}
	var requireClientCert *wrappers.BoolValue
	if common.ValidationContextType != nil {
		requireClientCert = &wrappers.BoolValue{Value: !dc.GetOneWayTls()}
	}
	// default alpn for downstreams.
	if len(common.AlpnProtocols) == 0 {
		common.AlpnProtocols = []string{"h2", "http/1.1"}
	}
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
	GetParameters() *v1.SslParameters
	GetAlpnProtocols() []string
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
	if sslSecrets.GetCallCredentials() != nil {
		// Deprecated way of building SDS. No longer used
		// by anything in gloo but still enabled for now
		// as it's a public API.
		return buildDeprecatedSDS(name, sslSecrets)
	}

	clusterName := defaultSdsClusterName
	if sslSecrets.GetClusterName() != "" {
		clusterName = sslSecrets.GetClusterName()
	}
	return &envoyauth.SdsSecretConfig{
		Name: name,
		SdsConfig: &envoycore.ConfigSource{
			ResourceApiVersion: envoycore.ApiVersion_V3,
			ConfigSourceSpecifier: &envoycore.ConfigSource_ApiConfigSource{
				ApiConfigSource: &envoycore.ApiConfigSource{
					ApiType:             envoycore.ApiConfigSource_GRPC,
					TransportApiVersion: envoycore.ApiVersion_V3,
					GrpcServices: []*envoycore.GrpcService{
						{
							TargetSpecifier: &envoycore.GrpcService_EnvoyGrpc_{
								EnvoyGrpc: &envoycore.GrpcService_EnvoyGrpc{
									ClusterName: clusterName,
								},
							},
						},
					},
				},
			},
		},
	}
}

func buildDeprecatedSDS(name string, sslSecrets *v1.SDSConfig) *envoyauth.SdsSecretConfig {
	config := &envoygrpccredential.FileBasedMetadataConfig{
		SecretData: &envoycore.DataSource{
			Specifier: &envoycore.DataSource_Filename{
				Filename: sslSecrets.GetCallCredentials().GetFileCredentialSource().GetTokenFileName(),
			},
		},
		HeaderKey: sslSecrets.GetCallCredentials().GetFileCredentialSource().GetHeader(),
	}
	any, _ := MessageToAny(config)

	gRPCConfig := &envoycore.GrpcService_GoogleGrpc{
		TargetUri:  sslSecrets.TargetUri,
		StatPrefix: "sds",
		ChannelCredentials: &envoycore.GrpcService_GoogleGrpc_ChannelCredentials{
			CredentialSpecifier: &envoycore.GrpcService_GoogleGrpc_ChannelCredentials_LocalCredentials{
				LocalCredentials: &envoycore.GrpcService_GoogleGrpc_GoogleLocalCredentials{},
			},
		},
		CredentialsFactoryName: MetadataPluginName,
		CallCredentials: []*envoycore.GrpcService_GoogleGrpc_CallCredentials{
			{
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
			ResourceApiVersion: envoycore.ApiVersion_V3,
			ConfigSourceSpecifier: &envoycore.ConfigSource_ApiConfigSource{
				ApiConfigSource: &envoycore.ApiConfigSource{
					ApiType:             envoycore.ApiConfigSource_GRPC,
					TransportApiVersion: envoycore.ApiVersion_V3,
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

func (s *sslConfigTranslator) handleSds(sslSecrets *v1.SDSConfig, matchSan []*envoymatcher.StringMatcher) (*envoyauth.CommonTlsContext, error) {
	if sslSecrets.CertificatesSecretName == "" && sslSecrets.ValidationContextName == "" {
		return nil, eris.Errorf("at least one of certificates_secret_name or validation_context_name must be provided")
	}
	if len(matchSan) != 0 && sslSecrets.ValidationContextName == "" {
		return nil, MissingValidationContextError
	}
	tlsContext := &envoyauth.CommonTlsContext{
		// default params
		TlsParams: &envoyauth.TlsParameters{},
	}

	if sslSecrets.CertificatesSecretName != "" {
		tlsContext.TlsCertificateSdsSecretConfigs = []*envoyauth.SdsSecretConfig{buildSds(sslSecrets.CertificatesSecretName, sslSecrets)}
	}

	if sslSecrets.ValidationContextName != "" {
		if len(matchSan) == 0 {
			tlsContext.ValidationContextType = &envoyauth.CommonTlsContext_ValidationContextSdsSecretConfig{
				ValidationContextSdsSecretConfig: buildSds(sslSecrets.ValidationContextName, sslSecrets),
			}
		} else {
			tlsContext.ValidationContextType = &envoyauth.CommonTlsContext_CombinedValidationContext{
				CombinedValidationContext: &envoyauth.CommonTlsContext_CombinedCertificateValidationContext{
					DefaultValidationContext:         &envoyauth.CertificateValidationContext{MatchSubjectAltNames: matchSan},
					ValidationContextSdsSecretConfig: buildSds(sslSecrets.ValidationContextName, sslSecrets),
				},
			}
		}
	}

	return tlsContext, nil
}

func (s *sslConfigTranslator) ResolveCommonSslConfig(cs CertSource, secrets v1.SecretList, mustHaveCert bool) (*envoyauth.CommonTlsContext, error) {
	var (
		certChain, privateKey, rootCa string
		// if using a Secret ref, we will inline the certs in the tls config
		inlineDataSource bool
	)

	if sslSecrets := cs.GetSecretRef(); sslSecrets != nil {
		var err error
		inlineDataSource = true
		ref := sslSecrets
		certChain, privateKey, rootCa, err = getSslSecrets(*ref, secrets)
		if err != nil {
			return nil, err
		}
	} else if sslSecrets := cs.GetSslFiles(); sslSecrets != nil {
		certChain, privateKey, rootCa = sslSecrets.TlsCert, sslSecrets.TlsKey, sslSecrets.RootCa
	} else if sslSecrets := cs.GetSds(); sslSecrets != nil {
		tlsContext, err := s.handleSds(sslSecrets, verifySanListToMatchSanList(cs.GetVerifySubjectAltName()))
		if err != nil {
			return nil, err
		}
		tlsContext.AlpnProtocols = cs.GetAlpnProtocols()
		return tlsContext, err
	} else {
		if mustHaveCert {
			return nil, NoCertificateFoundError
		}
	}

	if mustHaveCert {
		if certChain == "" || privateKey == "" {
			return nil, NoCertificateFoundError
		}
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
		return nil, eris.Errorf("both or none of cert chain and private key must be provided")
	}

	sanList := verifySanListToMatchSanList(cs.GetVerifySubjectAltName())

	if rootCaData != nil {
		validationCtx := &envoyauth.CommonTlsContext_ValidationContext{
			ValidationContext: &envoyauth.CertificateValidationContext{
				TrustedCa: rootCaData,
			},
		}
		if len(sanList) != 0 {
			validationCtx.ValidationContext.MatchSubjectAltNames = sanList
		}
		tlsContext.ValidationContextType = validationCtx

	} else if len(sanList) != 0 {
		return nil, RootCaMustBeProvidedError

	}

	var err error
	tlsContext.TlsParams, err = s.ResolveSslParamsConfig(cs.GetParameters())

	tlsContext.AlpnProtocols = cs.GetAlpnProtocols()
	return tlsContext, err
}

func getSslSecrets(ref core.ResourceRef, secrets v1.SecretList) (string, string, string, error) {
	secret, err := secrets.Find(ref.Strings())
	if err != nil {
		return "", "", "", SslSecretNotFoundError(err)
	}

	sslSecret, ok := secret.Kind.(*v1.Secret_Tls)
	if !ok {
		return "", "", "", NotTlsSecretError(secret.GetMetadata().Ref())
	}

	certChain := sslSecret.Tls.CertChain
	privateKey := sslSecret.Tls.PrivateKey
	rootCa := sslSecret.Tls.RootCa
	return certChain, privateKey, rootCa, nil
}

func (s *sslConfigTranslator) ResolveSslParamsConfig(params *v1.SslParameters) (*envoyauth.TlsParameters, error) {
	if params == nil {
		return nil, nil
	}

	maxver, err := convertVersion(params.MaximumProtocolVersion)
	if err != nil {
		return nil, err
	}
	minver, err := convertVersion(params.MinimumProtocolVersion)
	if err != nil {
		return nil, err
	}

	return &envoyauth.TlsParameters{
		CipherSuites:              params.CipherSuites,
		EcdhCurves:                params.EcdhCurves,
		TlsMaximumProtocolVersion: maxver,
		TlsMinimumProtocolVersion: minver,
	}, nil
}

func convertVersion(v v1.SslParameters_ProtocolVersion) (envoyauth.TlsParameters_TlsProtocol, error) {
	switch v {
	case v1.SslParameters_TLS_AUTO:
		return envoyauth.TlsParameters_TLS_AUTO, nil
	// TLS 1.0
	case v1.SslParameters_TLSv1_0:
		return envoyauth.TlsParameters_TLSv1_0, nil
	// TLS 1.1
	case v1.SslParameters_TLSv1_1:
		return envoyauth.TlsParameters_TLSv1_1, nil
	// TLS 1.2
	case v1.SslParameters_TLSv1_2:
		return envoyauth.TlsParameters_TLSv1_2, nil
	// TLS 1.3
	case v1.SslParameters_TLSv1_3:
		return envoyauth.TlsParameters_TLSv1_3, nil
	}

	return envoyauth.TlsParameters_TLS_AUTO, TlsVersionNotFoundError(v)
}

func verifySanListToMatchSanList(sanList []string) []*envoymatcher.StringMatcher {
	var matchSanList []*envoymatcher.StringMatcher
	for _, san := range sanList {
		matchSan := &envoymatcher.StringMatcher{
			MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: san},
		}
		matchSanList = append(matchSanList, matchSan)
	}
	return matchSanList
}
