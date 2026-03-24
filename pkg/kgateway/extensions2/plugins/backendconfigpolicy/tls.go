package backendconfigpolicy

import (
	"errors"
	"fmt"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoymatcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	eiutils "github.com/kgateway-dev/kgateway/v2/internal/envoyinit/pkg/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// SecretGetter defines the interface for retrieving secrets
type SecretGetter interface {
	GetSecret(name, namespace string) (*ir.Secret, error)
}

// DefaultSecretGetter implements SecretGetter using SecretIndex.GetSecretWithoutRefGrant
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
	return g.secrets.GetSecretWithoutRefGrant(g.krtctx, name, namespace)
}

func buildTLSContext(tlsConfig *kgateway.TLS, secretGetter SecretGetter, namespace string, tlsContext *envoytlsv3.CommonTlsContext) error {
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

func extractTLSData(tlsConfig *kgateway.TLS, secretGetter SecretGetter, namespace string) (*tlsData, error) {
	data := &tlsData{}

	if tlsConfig.SecretRef != nil {
		if err := extractFromSecret(tlsConfig.SecretRef, secretGetter, namespace, data); err != nil {
			return nil, err
		}
	} else if tlsConfig.Files != nil {
		extractFromFiles(tlsConfig.Files, data)
	}

	return data, nil
}

func extractFromSecret(secretRef *corev1.LocalObjectReference, secretGetter SecretGetter, namespace string, data *tlsData) error {
	secret, err := secretGetter.GetSecret(secretRef.Name, namespace)
	if err != nil {
		return err
	}

	data.certChain = string(secret.Data["tls.crt"])
	data.privateKey = string(secret.Data["tls.key"])
	data.rootCA = string(secret.Data["ca.crt"])
	data.inlineDataSource = true

	return nil
}

func extractFromFiles(tlsFiles *kgateway.TLSFiles, data *tlsData) {
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
	cleanedCertChain, err := pluginutils.CleanedSslKeyPair(tlsData.certChain, tlsData.privateKey)
	if err != nil {
		return fmt.Errorf("invalid certificate and key pair: %w", err)
	}

	var certChainData, privateKeyData *envoycorev3.DataSource
	if tlsData.inlineDataSource {
		certChainData = pluginutils.InlineStringDataSource(cleanedCertChain)
		privateKeyData = pluginutils.InlineStringDataSource(tlsData.privateKey)
	} else {
		certChainData = pluginutils.FileDataSource(cleanedCertChain)
		privateKeyData = pluginutils.FileDataSource(tlsData.privateKey)
	}

	tlsContext.TlsCertificates = []*envoytlsv3.TlsCertificate{
		{
			CertificateChain: certChainData,
			PrivateKey:       privateKeyData,
		},
	}

	return nil
}

func buildValidationContext(tlsData *tlsData, tlsConfig *kgateway.TLS, tlsContext *envoytlsv3.CommonTlsContext) error {
	sanMatchers := verifySanListToTypedMatchSanList(tlsConfig.VerifySubjectAltNames)

	// If the user opted to use the system CA bundle, configure a CombinedValidationContext
	// that references the SDS secret for the system CA set, and attach SAN matchers if any.
	if tlsConfig.WellKnownCACertificates != nil {
		switch *tlsConfig.WellKnownCACertificates {
		case gwv1.WellKnownCACertificatesSystem:
			combined := &envoytlsv3.CommonTlsContext_CombinedValidationContext{
				CombinedValidationContext: &envoytlsv3.CommonTlsContext_CombinedCertificateValidationContext{
					DefaultValidationContext: &envoytlsv3.CertificateValidationContext{
						MatchTypedSubjectAltNames: sanMatchers,
					},
					ValidationContextSdsSecretConfig: &envoytlsv3.SdsSecretConfig{
						Name: eiutils.SystemCaSecretName,
					},
				},
			}
			tlsContext.ValidationContextType = combined
			return nil
		default:
			logger.Error("unsupported WellKnownCACertificates value", "value", *tlsConfig.WellKnownCACertificates)
		}
	}

	if tlsData.rootCA == "" {
		// If no root CA and no SAN verification, no validation context needed
		if len(sanMatchers) == 0 {
			return nil
		}
		// Root CA is required if SAN verification is specified
		return errors.New("a root_ca must be provided if verify_subject_alt_name is not empty")
	}

	// If root CA is provided, build a validation context
	var rootCaData *envoycorev3.DataSource
	if tlsData.inlineDataSource {
		rootCaData = pluginutils.InlineStringDataSource(tlsData.rootCA)
	} else {
		rootCaData = pluginutils.FileDataSource(tlsData.rootCA)
	}

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
	tlsConfig *kgateway.TLS,
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

func parseTLSParameters(tlsParameters *kgateway.TLSParameters) (*envoytlsv3.TlsParameters, error) {
	if tlsParameters == nil {
		return nil, nil
	}

	maxVersion := ptr.Deref(tlsParameters.MaxVersion, kgateway.TLSVersionAUTO)
	minVersion := ptr.Deref(tlsParameters.MinVersion, kgateway.TLSVersionAUTO)

	tlsMaxVersion, err := parseTLSVersion(&maxVersion)
	if err != nil {
		return nil, err
	}
	tlsMinVersion, err := parseTLSVersion(&minVersion)
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

func parseTLSVersion(tlsVersion *kgateway.TLSVersion) (envoytlsv3.TlsParameters_TlsProtocol, error) {
	switch *tlsVersion {
	case kgateway.TLSVersion1_0:
		return envoytlsv3.TlsParameters_TLSv1_0, nil
	case kgateway.TLSVersion1_1:
		return envoytlsv3.TlsParameters_TLSv1_1, nil
	case kgateway.TLSVersion1_2:
		return envoytlsv3.TlsParameters_TLSv1_2, nil
	case kgateway.TLSVersion1_3:
		return envoytlsv3.TlsParameters_TLSv1_3, nil
	case kgateway.TLSVersionAUTO:
		return envoytlsv3.TlsParameters_TLS_AUTO, nil
	default:
		return 0, fmt.Errorf("invalid TLS version: %s", *tlsVersion)
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
