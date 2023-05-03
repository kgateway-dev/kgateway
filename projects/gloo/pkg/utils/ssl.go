package utils

import (
	"crypto/tls"
	"fmt"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoygrpccredential "github.com/envoyproxy/go-control-plane/envoy/config/grpc_credential/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoymatcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/constants"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

//go:generate mockgen -destination mocks/mock_ssl.go github.com/solo-io/gloo/projects/gloo/pkg/utils SslConfigTranslator

const (
	MetadataPluginName    = "envoy.grpc_credentials.file_based_metadata"
	defaultSdsClusterName = "gateway_proxy_sds"
)

var (
	TlsVersionNotFoundError = func(v ssl.SslParameters_ProtocolVersion) error {
		return eris.Errorf("tls version %v not found", v)
	}

	OcspStaplePolicyNotValidError = func(p ssl.SslConfig_OcspStaplePolicy) error {
		return eris.Errorf("ocsp staple policy %v not a valid policy", p)
	}

	SslSecretNotFoundError = func(err error) error {
		return eris.Wrapf(err, "SSL secret not found")
	}

	NotTlsSecretError = func(ref *core.ResourceRef) error {
		return eris.Errorf("%v is not a TLS secret", ref)
	}

	InvalidTlsSecretError = func(ref *core.ResourceRef, err error) error {
		if ref == nil {
			return eris.Wrapf(err, "Invalid TLS secret")
		} else {
			errorString := fmt.Sprintf("%v is not a valid TLS secret", ref)
			return eris.Wrapf(err, errorString)
		}
	}

	NoCertificateFoundError = eris.New("no certificate information found")

	MissingValidationContextError = eris.Errorf("must provide validation context name if verifying SAN")

	RootCaMustBeProvidedError = eris.Errorf("a root_ca must be provided if verify_subject_alt_name is not empty")
)

type SslConfigTranslator interface {
	ResolveUpstreamSslConfig(secrets v1.SecretList, uc *ssl.UpstreamSslConfig) (*envoyauth.UpstreamTlsContext, error)
	ResolveDownstreamSslConfig(secrets v1.SecretList, dc *ssl.SslConfig) (*envoyauth.DownstreamTlsContext, error)
	ResolveCommonSslConfig(cs CertSource, secrets v1.SecretList, mustHaveCert bool) (*envoyauth.CommonTlsContext, error)
	ResolveSslParamsConfig(params *ssl.SslParameters) (*envoyauth.TlsParameters, error)
}

type sslConfigTranslator struct {
}

func NewSslConfigTranslator() *sslConfigTranslator {
	return &sslConfigTranslator{}
}

func (s *sslConfigTranslator) ResolveUpstreamSslConfig(secrets v1.SecretList, uc *ssl.UpstreamSslConfig) (*envoyauth.UpstreamTlsContext, error) {
	common, err := s.ResolveCommonSslConfig(uc, secrets, false)
	if err != nil {
		return nil, err
	}
	return &envoyauth.UpstreamTlsContext{
		CommonTlsContext:   common,
		Sni:                uc.GetSni(),
		AllowRenegotiation: uc.GetAllowRenegotiation().GetValue(),
	}, nil
}

func (s *sslConfigTranslator) ResolveDownstreamSslConfig(secrets v1.SecretList, dc *ssl.SslConfig) (*envoyauth.DownstreamTlsContext, error) {
	common, err := s.ResolveCommonSslConfig(dc, secrets, true)
	if err != nil {
		return nil, err
	}
	var requireClientCert *wrappers.BoolValue
	if common.GetValidationContextType() != nil {
		requireClientCert = &wrappers.BoolValue{Value: !dc.GetOneWayTls().GetValue()}
	}

	// default alpn for downstreams.
	if len(common.GetAlpnProtocols()) == 0 {
		common.AlpnProtocols = []string{"h2", "http/1.1"}
	} else if len(common.GetAlpnProtocols()) == 1 && common.GetAlpnProtocols()[0] == constants.AllowEmpty { // allow override for advanced usage to set to a dangerous setting
		common.AlpnProtocols = []string{}

	}

	out := &envoyauth.DownstreamTlsContext{
		CommonTlsContext:         common,
		RequireClientCertificate: requireClientCert,
	}
	if dc.GetDisableTlsSessionResumption().GetValue() {
		out.SessionTicketKeysType = &envoyauth.DownstreamTlsContext_DisableStatelessSessionResumption{DisableStatelessSessionResumption: true}
	}
	ocspPolicy, err := convertOcspStaplePolicy(dc.GetOcspStaplePolicy())
	if err != nil {
		return nil, err
	}
	out.OcspStaplePolicy = ocspPolicy
	return out, nil
}

type CertSource interface {
	GetSecretRef() *core.ResourceRef
	GetSslFiles() *ssl.SSLFiles
	GetSds() *ssl.SDSConfig
	GetVerifySubjectAltName() []string
	GetParameters() *ssl.SslParameters
	GetAlpnProtocols() []string
}

// stringDataSourceGenerator returns a function that returns an Envoy data source that uses the given string as the data source.
// If inlineDataSource is false, the returned function returns a file data source. Otherwise, the returned function returns an inline-string data source.
func stringDataSourceGenerator(inlineDataSource bool) func(s string) *envoycore.DataSource {
	// Return a file data source if inlineDataSource is false.
	if !inlineDataSource {
		return func(s string) *envoycore.DataSource {
			return &envoycore.DataSource{
				Specifier: &envoycore.DataSource_Filename{
					Filename: s,
				},
			}
		}
	}

	return func(s string) *envoycore.DataSource {
		return &envoycore.DataSource{
			Specifier: &envoycore.DataSource_InlineString{
				InlineString: s,
			},
		}
	}
}

// byteDataSource returns an Envoy inline-bytes data source that uses the given byte slice as the data source.
func byteDataSource(b []byte) *envoycore.DataSource {
	return &envoycore.DataSource{
		Specifier: &envoycore.DataSource_InlineBytes{
			InlineBytes: b,
		},
	}
}

func buildSds(name string, sslSecrets *ssl.SDSConfig) *envoyauth.SdsSecretConfig {
	if sslSecrets.GetCallCredentials() != nil {
		// Deprecated way of building SDS. No longer used
		// by anything in gloo but still enabled for now
		// as it's a public API.
		return buildDeprecatedSDS(name, sslSecrets)
	}
	var grpcService *envoycore.GrpcService

	// If TargetUri is specified and ClusterName is not, create a GrpcService with a GoogleGrpc TargetSpecifier
	if targetUri := sslSecrets.GetTargetUri(); targetUri != "" && sslSecrets.GetClusterName() == "" {
		grpcService = &envoycore.GrpcService{
			TargetSpecifier: &envoycore.GrpcService_GoogleGrpc_{
				GoogleGrpc: &envoycore.GrpcService_GoogleGrpc{
					StatPrefix: name,
					TargetUri:  targetUri,
				},
			},
		}
		// Otherwise create a GrpcService with an EnvoyGrpc TargetSpecifier
	} else {
		clusterName := defaultSdsClusterName
		if sslSecrets.GetClusterName() != "" {
			clusterName = sslSecrets.GetClusterName()
		}

		grpcService = &envoycore.GrpcService{
			TargetSpecifier: &envoycore.GrpcService_EnvoyGrpc_{
				EnvoyGrpc: &envoycore.GrpcService_EnvoyGrpc{
					ClusterName: clusterName,
				},
			},
		}
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
						grpcService,
					},
				},
			},
		},
	}
}

func buildDeprecatedSDS(name string, sslSecrets *ssl.SDSConfig) *envoyauth.SdsSecretConfig {
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
		TargetUri:  sslSecrets.GetTargetUri(),
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

func (s *sslConfigTranslator) handleSds(sslSecrets *ssl.SDSConfig, matchSan []*envoymatcher.StringMatcher) (*envoyauth.CommonTlsContext, error) {
	if sslSecrets.GetCertificatesSecretName() == "" && sslSecrets.GetValidationContextName() == "" {
		return nil, eris.Errorf("at least one of certificates_secret_name or validation_context_name must be provided")
	}
	if len(matchSan) != 0 && sslSecrets.GetValidationContextName() == "" {
		return nil, MissingValidationContextError
	}
	tlsContext := &envoyauth.CommonTlsContext{
		// default params
		TlsParams: &envoyauth.TlsParameters{},
	}

	if sslSecrets.GetCertificatesSecretName() != "" {
		tlsContext.TlsCertificateSdsSecretConfigs = []*envoyauth.SdsSecretConfig{buildSds(sslSecrets.GetCertificatesSecretName(), sslSecrets)}
	}

	if sslSecrets.GetValidationContextName() != "" {
		if len(matchSan) == 0 {
			tlsContext.ValidationContextType = &envoyauth.CommonTlsContext_ValidationContextSdsSecretConfig{
				ValidationContextSdsSecretConfig: buildSds(sslSecrets.GetValidationContextName(), sslSecrets),
			}
		} else {
			tlsContext.ValidationContextType = &envoyauth.CommonTlsContext_CombinedValidationContext{
				CombinedValidationContext: &envoyauth.CommonTlsContext_CombinedCertificateValidationContext{
					DefaultValidationContext:         &envoyauth.CertificateValidationContext{MatchSubjectAltNames: matchSan},
					ValidationContextSdsSecretConfig: buildSds(sslSecrets.GetValidationContextName(), sslSecrets),
				},
			}
		}
	}

	return tlsContext, nil
}

func (s *sslConfigTranslator) ResolveCommonSslConfig(cs CertSource, secrets v1.SecretList, mustHaveCert bool) (*envoyauth.CommonTlsContext, error) {
	var (
		certChain, privateKey, rootCa, ocspStapleFile string
		// An OCSP response (staple) is a DER-encoded binary file
		ocspStaple []byte
		// if using a Secret ref, we will inline the certs in the tls config
		inlineDataSource bool
	)

	if sslSecrets := cs.GetSecretRef(); sslSecrets != nil {
		var err error
		inlineDataSource = true
		ref := sslSecrets
		certChain, privateKey, rootCa, ocspStaple, err = getSslSecrets(*ref, secrets)
		if err != nil {
			return nil, err
		}
	} else if sslFiles := cs.GetSslFiles(); sslFiles != nil {
		certChain, privateKey, rootCa = sslFiles.GetTlsCert(), sslFiles.GetTlsKey(), sslFiles.GetRootCa()
		// Since ocspStaple is []byte, but we want the file path, we're storing it in a separate string variable
		ocspStapleFile = sslFiles.GetOcspStaple()
		err := isValidSslKeyPair(certChain, privateKey, rootCa)
		if err != nil {
			return nil, InvalidTlsSecretError(nil, err)
		}
	} else if sslSds := cs.GetSds(); sslSds != nil {
		tlsContext, err := s.handleSds(sslSds, verifySanListToMatchSanList(cs.GetVerifySubjectAltName()))
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

	dataSource := stringDataSourceGenerator(inlineDataSource)

	var certChainData, privateKeyData, rootCaData, ocspStapleData *envoycore.DataSource

	if certChain != "" {
		certChainData = dataSource(certChain)
	}
	if privateKey != "" {
		privateKeyData = dataSource(privateKey)
	}
	if rootCa != "" {
		rootCaData = dataSource(rootCa)
	}
	// If we have a filename for the ocsp staple, we want to fetch ocsp data from the file otherwise, we use the []byte data stored in ocspStaple.
	if ocspStapleFile != "" {
		ocspStapleData = dataSource(ocspStapleFile)
	} else if ocspStaple != nil {
		ocspStapleData = byteDataSource(ocspStaple)
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
				OcspStaple:       ocspStapleData,
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

func getSslSecrets(ref core.ResourceRef, secrets v1.SecretList) (string, string, string, []byte, error) {
	secret, err := secrets.Find(ref.Strings())
	if err != nil {
		return "", "", "", nil, SslSecretNotFoundError(err)
	}

	sslSecret, ok := secret.GetKind().(*v1.Secret_Tls)
	if !ok {
		return "", "", "", nil, NotTlsSecretError(secret.GetMetadata().Ref())
	}

	certChain := sslSecret.Tls.GetCertChain()
	privateKey := sslSecret.Tls.GetPrivateKey()
	rootCa := sslSecret.Tls.GetRootCa()
	ocspStaple := sslSecret.Tls.GetOcspStaple()

	err = isValidSslKeyPair(certChain, privateKey, rootCa)
	if err != nil {
		return "", "", "", nil, InvalidTlsSecretError(secret.GetMetadata().Ref(), err)
	}

	return certChain, privateKey, rootCa, ocspStaple, nil
}

func isValidSslKeyPair(certChain, privateKey, rootCa string) error {
	// in the case where we _only_ provide a rootCa, we do not want to validate tls.key+tls.cert
	if (certChain == "") && (privateKey == "") && (rootCa != "") {
		return nil
	}

	_, err := tls.X509KeyPair([]byte(certChain), []byte(privateKey))
	return err
}

func (s *sslConfigTranslator) ResolveSslParamsConfig(params *ssl.SslParameters) (*envoyauth.TlsParameters, error) {
	if params == nil {
		return nil, nil
	}

	maxver, err := convertVersion(params.GetMaximumProtocolVersion())
	if err != nil {
		return nil, err
	}
	minver, err := convertVersion(params.GetMinimumProtocolVersion())
	if err != nil {
		return nil, err
	}

	return &envoyauth.TlsParameters{
		CipherSuites:              params.GetCipherSuites(),
		EcdhCurves:                params.GetEcdhCurves(),
		TlsMaximumProtocolVersion: maxver,
		TlsMinimumProtocolVersion: minver,
	}, nil
}

func convertVersion(v ssl.SslParameters_ProtocolVersion) (envoyauth.TlsParameters_TlsProtocol, error) {
	switch v {
	case ssl.SslParameters_TLS_AUTO:
		return envoyauth.TlsParameters_TLS_AUTO, nil
	// TLS 1.0
	case ssl.SslParameters_TLSv1_0:
		return envoyauth.TlsParameters_TLSv1_0, nil
	// TLS 1.1
	case ssl.SslParameters_TLSv1_1:
		return envoyauth.TlsParameters_TLSv1_1, nil
	// TLS 1.2
	case ssl.SslParameters_TLSv1_2:
		return envoyauth.TlsParameters_TLSv1_2, nil
	// TLS 1.3
	case ssl.SslParameters_TLSv1_3:
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

func convertOcspStaplePolicy(policy ssl.SslConfig_OcspStaplePolicy) (envoyauth.DownstreamTlsContext_OcspStaplePolicy, error) {
	switch policy {
	case ssl.SslConfig_LENIENT_STAPLING:
		return envoyauth.DownstreamTlsContext_LENIENT_STAPLING, nil
	case ssl.SslConfig_STRICT_STAPLING:
		return envoyauth.DownstreamTlsContext_STRICT_STAPLING, nil
	case ssl.SslConfig_MUST_STAPLE:
		return envoyauth.DownstreamTlsContext_MUST_STAPLE, nil
	default:
		// This should not occur. An invalid value should default to LENIENT_STAPLING.
		return envoyauth.DownstreamTlsContext_LENIENT_STAPLING, OcspStaplePolicyNotValidError(policy)
	}
}
