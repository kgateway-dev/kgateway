package utils

import (
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

// We support global UpstreamOptions to define SslParameters for all upstreams
// If an upstream is configure with ssl, it will inherit the defaults here:
// https://github.com/solo-io/gloo/blob/15da82bdd65ab4bcedbc7fb803ea0bb5f7e926fc/projects/gloo/pkg/translator/clusters.go#L108
// However, if an upstream is configured with one-way TLS, we must explicitly apply the defaults, since there is no ssl
// configuration on the upstream
func GetCommonTlsContextFromUpstreamOptions(options *v1.UpstreamOptions) (*envoyauth.CommonTlsContext, error) {
	sslCfgTranslator := NewSslConfigTranslator()
	tlsParams, err := sslCfgTranslator.ResolveSslParamsConfig(options.GetSslParameters())

	return &envoyauth.CommonTlsContext{
		TlsParams: tlsParams,
	}, err
}
