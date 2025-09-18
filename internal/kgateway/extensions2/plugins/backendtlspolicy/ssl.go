package backendtlspolicy

import (
	"errors"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	corev1 "k8s.io/api/core/v1"
)

// handles conversion into envoy auth types
// based on https://github.com/solo-io/gloo/blob/main/projects/gloo/pkg/utils/ssl.go#L76

var noKeyFoundMsg = "no key ca.crt found"

func ResolveUpstreamSslConfig(cm *corev1.ConfigMap, validation *envoytlsv3.CertificateValidationContext, sni string) (*envoytlsv3.UpstreamTlsContext, error) {
	common, err := ResolveCommonSslConfig(cm, validation, false)
	if err != nil {
		return nil, err
	}

	return &envoytlsv3.UpstreamTlsContext{
		CommonTlsContext: common,
		Sni:              sni,
	}, nil
}

func ResolveCommonSslConfig(cm *corev1.ConfigMap, validation *envoytlsv3.CertificateValidationContext, mustHaveCert bool) (*envoytlsv3.CommonTlsContext, error) {
	caCrt, err := getSslSecrets(cm)
	if err != nil {
		return nil, err
	}

	// TODO: should we do some validation on the CA?
	caCrtData := envoycorev3.DataSource{
		Specifier: &envoycorev3.DataSource_InlineString{
			InlineString: caCrt,
		},
	}

	tlsContext := &envoytlsv3.CommonTlsContext{
		// default params
		TlsParams: &envoytlsv3.TlsParameters{},
	}
	validation.TrustedCa = &caCrtData
	validationCtx := &envoytlsv3.CommonTlsContext_ValidationContext{
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
