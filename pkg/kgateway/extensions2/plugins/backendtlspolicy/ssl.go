package backendtlspolicy

import (
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
)

// handles conversion into envoy auth types
// based on https://github.com/solo-io/gloo/blob/main/projects/gloo/pkg/utils/ssl.go#L76

// ResolveUpstreamSslConfigFromCA creates an UpstreamTlsContext from a CA certificate string.
// This delegates to the shared implementation in pluginutils.
func ResolveUpstreamSslConfigFromCA(caCert string, validation *envoytlsv3.CertificateValidationContext, sni string) (*envoytlsv3.UpstreamTlsContext, error) {
	return pluginutils.ResolveUpstreamSslConfigFromCA(caCert, validation, sni)
}
