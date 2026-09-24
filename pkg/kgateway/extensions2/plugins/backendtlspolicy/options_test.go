package backendtlspolicy

import (
	"testing"

	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/annotations"
)

// newSystemCABackendTLSPolicy builds a minimal BackendTLSPolicy that uses the system CA bundle
// for validation, with the given extension options set.
func newSystemCABackendTLSPolicy(options map[gwv1.AnnotationKey]gwv1.AnnotationValue) *gwv1.BackendTLSPolicy {
	return &gwv1.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "policy",
			Namespace: "default",
		},
		Spec: gwv1.BackendTLSPolicySpec{
			Validation: gwv1.BackendTLSPolicyValidation{
				WellKnownCACertificates: ptr.To(gwv1.WellKnownCACertificatesSystem),
				Hostname:                "example.com",
			},
			Options: options,
		},
	}
}

func TestBuildTranslateFunc_AppliesTLSExtensionOptions(t *testing.T) {
	translate := buildTranslateFunc(nil, nil)

	policy := newSystemCABackendTLSPolicy(map[gwv1.AnnotationKey]gwv1.AnnotationValue{
		annotations.EcdhCurves:            "P-384,P-256",
		annotations.CipherSuites:          "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
		annotations.SignatureAlgorithms:   "ecdsa_secp256r1_sha256",
		annotations.MinTLSVersion:         "1.2",
		annotations.MaxTLSVersion:         "1.3",
		annotations.AlpnProtocols:         "h2,http/1.1",
		annotations.VerifyCertificateHash: "7D86C6654C8229364ECFE4D4964C69410090AE09E9B4D0C9B2AD7854175AD51D",
	})

	pol, err := translate(krt.TestingDummyContext{}, policy)
	require.NoError(t, err)

	tlsCtx := &envoytlsv3.UpstreamTlsContext{}
	require.NoError(t, pol.transportSocket.GetTypedConfig().UnmarshalTo(tlsCtx))

	common := tlsCtx.GetCommonTlsContext()
	require.NotNil(t, common.GetTlsParams())
	assert.Equal(t, []string{"P-384", "P-256"}, common.GetTlsParams().GetEcdhCurves())
	assert.Equal(t, []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"}, common.GetTlsParams().GetCipherSuites())
	assert.Equal(t, []string{"ecdsa_secp256r1_sha256"}, common.GetTlsParams().GetSignatureAlgorithms())
	assert.Equal(t, envoytlsv3.TlsParameters_TLSv1_2, common.GetTlsParams().GetTlsMinimumProtocolVersion())
	assert.Equal(t, envoytlsv3.TlsParameters_TLSv1_3, common.GetTlsParams().GetTlsMaximumProtocolVersion())
	assert.Equal(t, []string{"h2", "http/1.1"}, common.GetAlpnProtocols())

	validationCtx := common.GetCombinedValidationContext().GetDefaultValidationContext()
	require.NotNil(t, validationCtx)
	assert.Equal(t, []string{"7D86C6654C8229364ECFE4D4964C69410090AE09E9B4D0C9B2AD7854175AD51D"}, validationCtx.GetVerifyCertificateHash())

	// Only the mandatory SAN matcher from spec.Validation.Hostname: verify-subject-alt-names is
	// rejected for BackendTLSPolicy, so it can never widen this set.
	require.Len(t, validationCtx.GetMatchTypedSubjectAltNames(), 1)
	assert.Equal(t, "example.com", validationCtx.GetMatchTypedSubjectAltNames()[0].GetMatcher().GetExact())
}

func TestBuildTranslateFunc_VerifySubjectAltNamesRejected(t *testing.T) {
	translate := buildTranslateFunc(nil, nil)

	// verify-subject-alt-names would OR extra SANs into the match set, letting a certificate
	// that lacks the required hostname still pass validation, so it must be rejected outright.
	policy := newSystemCABackendTLSPolicy(map[gwv1.AnnotationKey]gwv1.AnnotationValue{
		annotations.VerifySubjectAltNames: "extra.example.com",
	})

	_, err := translate(krt.TestingDummyContext{}, policy)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVerifySubjectAltNamesNotSupported)
}

func TestBuildTranslateFunc_MinTLSVersionAloneNormalizesMax(t *testing.T) {
	translate := buildTranslateFunc(nil, nil)

	// A minimum alone must not leave the effective maximum below it (e.g. at Envoy's own
	// implicit default), which would produce an inverted, unusable range.
	policy := newSystemCABackendTLSPolicy(map[gwv1.AnnotationKey]gwv1.AnnotationValue{
		annotations.MinTLSVersion: "1.3",
	})

	pol, err := translate(krt.TestingDummyContext{}, policy)
	require.NoError(t, err)

	tlsCtx := &envoytlsv3.UpstreamTlsContext{}
	require.NoError(t, pol.transportSocket.GetTypedConfig().UnmarshalTo(tlsCtx))

	tlsParams := tlsCtx.GetCommonTlsContext().GetTlsParams()
	require.NotNil(t, tlsParams)
	assert.Equal(t, envoytlsv3.TlsParameters_TLSv1_3, tlsParams.GetTlsMinimumProtocolVersion())
	assert.Equal(t, envoytlsv3.TlsParameters_TLSv1_3, tlsParams.GetTlsMaximumProtocolVersion())
}

func TestBuildTranslateFunc_InvalidTLSOption(t *testing.T) {
	translate := buildTranslateFunc(nil, nil)

	policy := newSystemCABackendTLSPolicy(map[gwv1.AnnotationKey]gwv1.AnnotationValue{
		annotations.MinTLSVersion: "not-a-version",
	})

	_, err := translate(krt.TestingDummyContext{}, policy)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidTLSOptions)
}

func TestBuildTranslateFunc_AllowEmptyAlpnProtocols(t *testing.T) {
	translate := buildTranslateFunc(nil, nil)

	policy := newSystemCABackendTLSPolicy(map[gwv1.AnnotationKey]gwv1.AnnotationValue{
		annotations.AlpnProtocols: annotations.AllowEmptyAlpnProtocols,
	})

	pol, err := translate(krt.TestingDummyContext{}, policy)
	require.NoError(t, err)

	tlsCtx := &envoytlsv3.UpstreamTlsContext{}
	require.NoError(t, pol.transportSocket.GetTypedConfig().UnmarshalTo(tlsCtx))
	assert.Empty(t, tlsCtx.GetCommonTlsContext().GetAlpnProtocols())
}

func TestBuildTranslateFunc_NoOptions(t *testing.T) {
	translate := buildTranslateFunc(nil, nil)

	policy := newSystemCABackendTLSPolicy(nil)

	pol, err := translate(krt.TestingDummyContext{}, policy)
	require.NoError(t, err)

	tlsCtx := &envoytlsv3.UpstreamTlsContext{}
	require.NoError(t, pol.transportSocket.GetTypedConfig().UnmarshalTo(tlsCtx))
	assert.Nil(t, tlsCtx.GetCommonTlsContext().GetTlsParams())
}
