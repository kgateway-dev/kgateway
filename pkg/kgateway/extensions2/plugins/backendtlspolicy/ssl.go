package backendtlspolicy

import (
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	corev1 "k8s.io/api/core/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/sslutils"
)

// handles conversion into envoy auth types
// based on https://github.com/solo-io/gloo/blob/main/projects/gloo/pkg/utils/ssl.go#L76

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
	caCrt, err := sslutils.GetCACertFromConfigMap(cm)
	if err != nil {
		return nil, err
	}

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
