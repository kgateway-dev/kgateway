package backendconfigpolicy

import (
	"crypto/tls"
	"errors"
	"fmt"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoymatcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"

	"istio.io/istio/pkg/kube/krt"
	"k8s.io/client-go/util/cert"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
)

// SecretGetter defines the interface for retrieving secrets
type SecretGetter interface {
	GetSecret(name, namespace string) (*ir.Secret, error)
}

// DefaultSecretGetter implements SecretGetter using the pluginutils.GetSecretIr function
type DefaultSecretGetter struct {
	secrets *krtcollections.SecretIndex
	krtctx  krt.HandlerContext
}

func NewDefaultSecretGetter(secrets *krtcollections.SecretIndex, krtctx krt.HandlerContext) *DefaultSecretGetter {
	return &DefaultSecretGetter{
		secrets: secrets,
		krtctx:  krtctx,
	}
}

func (g *DefaultSecretGetter) GetSecret(name, namespace string) (*ir.Secret, error) {
	return pluginutils.GetSecretIr(g.secrets, g.krtctx, name, namespace)
}

func translateTLSConfig(
	secretGetter SecretGetter,
	tlsConfig *v1alpha1.TLS,
	namespace string,
) (*envoyauth.UpstreamTlsContext, error) {
	var (
		certChain, privateKey, rootCA string
		inlineDataSource              bool
	)
	if tlsConfig.SecretRef != nil {
		secret, err := secretGetter.GetSecret(tlsConfig.SecretRef.Name, namespace)
		if err != nil {
			return nil, err
		}
		certChain = string(secret.Data["tls.crt"])
		privateKey = string(secret.Data["tls.key"])
		rootCA = string(secret.Data["ca.crt"])
		inlineDataSource = true
	} else if tlsConfig.TLSFiles != nil {
		certChain = tlsConfig.TLSFiles.TLSCertificate
		privateKey = tlsConfig.TLSFiles.TLSKey
		rootCA = tlsConfig.TLSFiles.RootCA
	}

	cleanedCertChain, err := cleanedSslKeyPair(certChain, privateKey, rootCA)
	if err != nil {
		return nil, err
	}

	dataSource := stringDataSourceGenerator(inlineDataSource)

	var certChainData, privateKeyData, rootCaData *corev3.DataSource
	if cleanedCertChain != "" {
		certChainData = dataSource(cleanedCertChain)
	}
	if privateKey != "" {
		privateKeyData = dataSource(privateKey)
	}
	if rootCA != "" {
		rootCaData = dataSource(rootCA)
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
		return nil, errors.New("invalid TLS config: certChain and privateKey must both be provided")
	}

	sanList := verifySanListToMatchSanList(tlsConfig.VerifySubjectAltName)

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
		return nil, errors.New("a root_ca must be provided if verify_subject_alt_name is not empty")
	}

	tlsParams, err := parseTLSParameters(tlsConfig.Parameters)
	if err != nil {
		return nil, err
	}
	tlsContext.TlsParams = tlsParams

	if tlsConfig.OneWayTLS != nil && *tlsConfig.OneWayTLS {
		tlsContext.ValidationContextType = nil
	}

	if tlsConfig.AlpnProtocols != nil {
		tlsContext.AlpnProtocols = tlsConfig.AlpnProtocols
	}

	return &envoyauth.UpstreamTlsContext{
		CommonTlsContext:   tlsContext,
		Sni:                tlsConfig.Sni,
		AllowRenegotiation: ptr.Deref(tlsConfig.AllowRenegotiation, false),
	}, nil
}

func parseTLSParameters(tlsParameters *v1alpha1.Parameters) (*envoyauth.TlsParameters, error) {
	if tlsParameters == nil {
		return nil, nil
	}

	tlsMaxVersion, err := parseTLSVersion(tlsParameters.TLSMaxVersion)
	if err != nil {
		return nil, err
	}
	tlsMinVersion, err := parseTLSVersion(tlsParameters.TLSMinVersion)
	if err != nil {
		return nil, err
	}

	return &envoyauth.TlsParameters{
		CipherSuites:              tlsParameters.CipherSuites,
		EcdhCurves:                tlsParameters.EcdhCurves,
		TlsMinimumProtocolVersion: tlsMinVersion,
		TlsMaximumProtocolVersion: tlsMaxVersion,
	}, nil
}

func parseTLSVersion(tlsVersion *v1alpha1.TLSVersion) (envoyauth.TlsParameters_TlsProtocol, error) {
	switch *tlsVersion {
	case v1alpha1.TLSVersion1_0:
		return envoyauth.TlsParameters_TLSv1_0, nil
	case v1alpha1.TLSVersion1_1:
		return envoyauth.TlsParameters_TLSv1_1, nil
	case v1alpha1.TLSVersion1_2:
		return envoyauth.TlsParameters_TLSv1_2, nil
	case v1alpha1.TLSVersion1_3:
		return envoyauth.TlsParameters_TLSv1_3, nil
	case v1alpha1.TLSVersionAUTO:
		return envoyauth.TlsParameters_TLS_AUTO, nil
	default:
		return 0, fmt.Errorf("invalid TLS version: %s", *tlsVersion)
	}
}

func cleanedSslKeyPair(certChain, privateKey, rootCa string) (cleanedChain string, err error) {
	// in the case where we _only_ provide a rootCa, we do not want to validate tls.key+tls.cert
	if (certChain == "") && (privateKey == "") && (rootCa != "") {
		return certChain, nil
	}

	// validate that the cert and key are a valid pair
	_, err = tls.X509KeyPair([]byte(certChain), []byte(privateKey))
	if err != nil {
		return "", err
	}

	// validate that the parsed piece is valid
	// this is still faster than a call out to openssl despite this second parsing pass of the cert
	// pem parsing in go is permissive while envoy is not
	// this might not be needed once we have larger envoy validation
	candidateCert, err := cert.ParseCertsPEM([]byte(certChain))
	if err != nil {
		// return err rather than sanitize. This is to maintain UX with older versions and to keep in line with gateway2 pkg.
		return "", err
	}
	cleanedChainBytes, err := cert.EncodeCertificates(candidateCert...)
	cleanedChain = string(cleanedChainBytes)

	return cleanedChain, err
}

// stringDataSourceGenerator returns a function that returns an Envoy data source that uses the given string as the data source.
// If inlineDataSource is false, the returned function returns a file data source. Otherwise, the returned function returns an inline-string data source.
func stringDataSourceGenerator(inlineDataSource bool) func(s string) *corev3.DataSource {
	// Return a file data source if inlineDataSource is false.
	if !inlineDataSource {
		return func(s string) *corev3.DataSource {
			return &corev3.DataSource{
				Specifier: &corev3.DataSource_Filename{
					Filename: s,
				},
			}
		}
	}

	return func(s string) *corev3.DataSource {
		return &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: s,
			},
		}
	}
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
