package backendconfigpolicy

import (
	"crypto/tls"
	"errors"
	"fmt"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoymatcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"

	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
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

func buildTLSContext(tlsConfig *v1alpha1.TLS, secretGetter SecretGetter, namespace string, tlsContext *envoytlsv3.CommonTlsContext) error {
	// Extract TLS data from config
	tlsData, err := extractTLSData(tlsConfig, secretGetter, namespace)
	if err != nil {
		return fmt.Errorf("failed to extract TLS data: %w", err)
	}

	// Skip client certificate processing for simple TLS
	if tlsConfig.SimpleTLS != nil && *tlsConfig.SimpleTLS {
		return buildValidationContext(tlsData, tlsConfig, tlsContext)
	}

	// Process client certificate for mutual TLS, if provided
	if err := buildCertificateContext(tlsData, tlsContext); err != nil {
		return err
	}

	return buildValidationContext(tlsData, tlsConfig, tlsContext)
}

type tlsData struct {
	certChain        string
	privateKey       string
	rootCA           string
	inlineDataSource bool
}

func extractTLSData(tlsConfig *v1alpha1.TLS, secretGetter SecretGetter, namespace string) (*tlsData, error) {
	data := &tlsData{}

	if tlsConfig.SecretRef != nil {
		if err := extractFromSecret(tlsConfig.SecretRef, secretGetter, namespace, data); err != nil {
			return nil, err
		}
	} else if tlsConfig.TLSFiles != nil {
		extractFromFiles(tlsConfig.TLSFiles, data)
	} else {
		return nil, errors.New("either SecretRef or TLSFiles must be provided")
	}

	return data, nil
}

func extractFromSecret(secretRef *corev1.LocalObjectReference, secretGetter SecretGetter, namespace string, data *tlsData) error {
	secret, err := secretGetter.GetSecret(secretRef.Name, namespace)
	if err != nil {
		return fmt.Errorf("failed to get secret %s: %w", secretRef.Name, err)
	}

	data.certChain = string(secret.Data["tls.crt"])
	data.privateKey = string(secret.Data["tls.key"])
	data.rootCA = string(secret.Data["ca.crt"])
	data.inlineDataSource = true

	return nil
}

func extractFromFiles(tlsFiles *v1alpha1.TLSFiles, data *tlsData) {
	data.certChain = ptr.Deref(tlsFiles.TLSCertificate, "")
	data.privateKey = ptr.Deref(tlsFiles.TLSKey, "")
	data.rootCA = ptr.Deref(tlsFiles.RootCA, "")
	data.inlineDataSource = false
}

func buildCertificateContext(tlsData *tlsData, tlsContext *envoytlsv3.CommonTlsContext) error {
	// For mTLS, both the certificate chain and the private key are required.
	// If neither is provided, we assume mTLS is not intended, so we can exit early.
	if tlsData.certChain == "" && tlsData.privateKey == "" {
		return nil
	}

	// If one is provided without the other, it's a configuration error.
	if tlsData.certChain == "" || tlsData.privateKey == "" {
		return errors.New("invalid TLS config: for if providing a client certificate, both certChain and privateKey must be provided")
	}

	// Validate the certificate and key pair, and get a sanitized version of the certificate chain.
	cleanedCertChain, err := cleanedSslKeyPair(tlsData.certChain, tlsData.privateKey)
	if err != nil {
		return fmt.Errorf("invalid certificate and key pair: %w", err)
	}

	dataSource := stringDataSourceGenerator(tlsData.inlineDataSource)

	certChainData := dataSource(cleanedCertChain)
	privateKeyData := dataSource(tlsData.privateKey)

	tlsContext.TlsCertificates = []*envoytlsv3.TlsCertificate{
		{
			CertificateChain: certChainData,
			PrivateKey:       privateKeyData,
		},
	}

	return nil
}

func buildValidationContext(tlsData *tlsData, tlsConfig *v1alpha1.TLS, tlsContext *envoytlsv3.CommonTlsContext) error {
	sanMatchers := verifySanListToTypedMatchSanList(tlsConfig.VerifySubjectAltName)

	if tlsData.rootCA == "" {
		// If no root CA and no SAN verification, no validation context needed
		if len(sanMatchers) == 0 {
			return nil
		}
		// Root CA is required if SAN verification is specified
		return errors.New("a root_ca must be provided if verify_subject_alt_name is not empty")
	}

	// If root CA is provided, build a validation context
	dataSource := stringDataSourceGenerator(tlsData.inlineDataSource)
	rootCaData := dataSource(tlsData.rootCA)

	validationCtx := &envoytlsv3.CommonTlsContext_ValidationContext{
		ValidationContext: &envoytlsv3.CertificateValidationContext{
			TrustedCa: rootCaData,
		},
	}
	if len(sanMatchers) > 0 {
		validationCtx.ValidationContext.MatchTypedSubjectAltNames = sanMatchers
	}
	tlsContext.ValidationContextType = validationCtx

	return nil
}

func translateTLSConfig(
	secretGetter SecretGetter,
	tlsConfig *v1alpha1.TLS,
	namespace string,
) (*envoytlsv3.UpstreamTlsContext, error) {
	tlsContext := &envoytlsv3.CommonTlsContext{
		TlsParams: &envoytlsv3.TlsParameters{}, // default params
	}

	tlsParams, err := parseTLSParameters(tlsConfig.Parameters)
	if err != nil {
		return nil, err
	}
	tlsContext.TlsParams = tlsParams

	if tlsConfig.AlpnProtocols != nil {
		tlsContext.AlpnProtocols = tlsConfig.AlpnProtocols
	}

	if tlsConfig.InsecureSkipVerify != nil && *tlsConfig.InsecureSkipVerify {
		tlsContext.ValidationContextType = &envoytlsv3.CommonTlsContext_ValidationContext{}
	} else {
		if err := buildTLSContext(tlsConfig, secretGetter, namespace, tlsContext); err != nil {
			return nil, err
		}
	}

	return &envoytlsv3.UpstreamTlsContext{
		CommonTlsContext:   tlsContext,
		Sni:                ptr.Deref(tlsConfig.Sni, ""),
		AllowRenegotiation: ptr.Deref(tlsConfig.AllowRenegotiation, false),
	}, nil
}

func parseTLSParameters(tlsParameters *v1alpha1.Parameters) (*envoytlsv3.TlsParameters, error) {
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

	return &envoytlsv3.TlsParameters{
		CipherSuites:              tlsParameters.CipherSuites,
		EcdhCurves:                tlsParameters.EcdhCurves,
		TlsMinimumProtocolVersion: tlsMinVersion,
		TlsMaximumProtocolVersion: tlsMaxVersion,
	}, nil
}

func parseTLSVersion(tlsVersion *v1alpha1.TLSVersion) (envoytlsv3.TlsParameters_TlsProtocol, error) {
	switch *tlsVersion {
	case v1alpha1.TLSVersion1_0:
		return envoytlsv3.TlsParameters_TLSv1_0, nil
	case v1alpha1.TLSVersion1_1:
		return envoytlsv3.TlsParameters_TLSv1_1, nil
	case v1alpha1.TLSVersion1_2:
		return envoytlsv3.TlsParameters_TLSv1_2, nil
	case v1alpha1.TLSVersion1_3:
		return envoytlsv3.TlsParameters_TLSv1_3, nil
	case v1alpha1.TLSVersionAUTO:
		return envoytlsv3.TlsParameters_TLS_AUTO, nil
	default:
		return 0, fmt.Errorf("invalid TLS version: %s", *tlsVersion)
	}
}

func cleanedSslKeyPair(certChain, privateKey string) (cleanedChain string, err error) {
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
func stringDataSourceGenerator(inlineDataSource bool) func(s string) *envoycorev3.DataSource {
	// Return a file data source if inlineDataSource is false.
	if !inlineDataSource {
		return func(s string) *envoycorev3.DataSource {
			return &envoycorev3.DataSource{
				Specifier: &envoycorev3.DataSource_Filename{
					Filename: s,
				},
			}
		}
	}

	return func(s string) *envoycorev3.DataSource {
		return &envoycorev3.DataSource{
			Specifier: &envoycorev3.DataSource_InlineString{
				InlineString: s,
			},
		}
	}
}

func verifySanListToTypedMatchSanList(sanList []string) []*envoytlsv3.SubjectAltNameMatcher {
	var matchSanList []*envoytlsv3.SubjectAltNameMatcher
	for _, san := range sanList {
		matchSan := &envoytlsv3.SubjectAltNameMatcher{
			SanType: envoytlsv3.SubjectAltNameMatcher_DNS,
			Matcher: &envoymatcher.StringMatcher{
				MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: san},
			},
		}
		matchSanList = append(matchSanList, matchSan)
	}
	return matchSanList
}
