package backendtlspolicy

import (
	"errors"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoymatcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	corev1 "k8s.io/api/core/v1"
)

// handles conversion into envoy auth types
// based on https://github.com/solo-io/gloo/blob/main/projects/gloo/pkg/utils/ssl.go#L76

var noKeyFoundMsg = "no key ca.crt found"

func ResolveUpstreamSslConfig(cm *corev1.ConfigMap, sni string) (*envoyauth.UpstreamTlsContext, error) {
	common, err := ResolveCommonSslConfig(cm, false)
	if err != nil {
		return nil, err
	}

	return &envoyauth.UpstreamTlsContext{
		CommonTlsContext: common,
		Sni:              sni,
	}, nil
}

func ResolveCommonSslConfig(cm *corev1.ConfigMap, mustHaveCert bool) (*envoyauth.CommonTlsContext, error) {
	caCrt, err := getSslSecrets(cm)
	if err != nil {
		return nil, err
	}

	// TODO: should we do some validation on the CA?
	caCrtData := envoycore.DataSource{
		Specifier: &envoycore.DataSource_InlineString{
			InlineString: caCrt,
		},
	}

	tlsContext := &envoyauth.CommonTlsContext{
		// default params
		TlsParams: &envoyauth.TlsParameters{},
	}

	validationCtx := &envoyauth.CommonTlsContext_ValidationContext{
		ValidationContext: &envoyauth.CertificateValidationContext{
			TrustedCa: &caCrtData,
		},
	}
	// sanList := VerifySanListToMatchSanList(cs.GetVerifySubjectAltName())
	// if len(sanList) != 0 {
	// 	validationCtx.ValidationContext.MatchSubjectAltNames = sanList
	// }
	tlsContext.ValidationContextType = validationCtx
	return tlsContext, nil
}

func getSslSecrets(cm *corev1.ConfigMap) (string, error) {
	caCrt, ok := cm.Data["ca.crt"]
	if !ok {
		return "", errors.New(noKeyFoundMsg)
	}

	return caCrt, nil
}

func VerifySanListToMatchSanList(sanList []string) []*envoymatcher.StringMatcher {
	var matchSanList []*envoymatcher.StringMatcher
	for _, san := range sanList {
		matchSan := &envoymatcher.StringMatcher{
			MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: san},
		}
		matchSanList = append(matchSanList, matchSan)
	}
	return matchSanList
}
