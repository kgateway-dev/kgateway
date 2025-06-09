package backendtlspolicy

import (
	"errors"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	corev1 "k8s.io/api/core/v1"
)

// handles conversion into envoy auth types
// based on https://github.com/solo-io/gloo/blob/main/projects/gloo/pkg/utils/ssl.go#L76

var noKeyFoundMsg = "no key ca.crt found"

func ResolveUpstreamSslConfig(cm *corev1.ConfigMap, validation *envoyauth.CertificateValidationContext, sni string) (*envoyauth.UpstreamTlsContext, error) {
	common, err := ResolveCommonSslConfig(cm, validation, false)
	if err != nil {
		return nil, err
	}

	return &envoyauth.UpstreamTlsContext{
		CommonTlsContext: common,
		Sni:              sni,
	}, nil
}

func ResolveCommonSslConfig(cm *corev1.ConfigMap, validation *envoyauth.CertificateValidationContext, mustHaveCert bool) (*envoyauth.CommonTlsContext, error) {
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
	validation.TrustedCa = &caCrtData
	validationCtx := &envoyauth.CommonTlsContext_ValidationContext{
		ValidationContext: validation,
	}

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
