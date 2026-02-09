package pluginutils

import (
	"crypto/tls"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"k8s.io/client-go/util/cert"
)

// ResolveUpstreamSslConfigFromCA creates an UpstreamTlsContext from a CA certificate string.
func ResolveUpstreamSslConfigFromCA(caCert string, validation *envoytlsv3.CertificateValidationContext, sni string) (*envoytlsv3.UpstreamTlsContext, error) {
	common, err := ResolveCommonSslConfigFromCA(caCert, validation)
	if err != nil {
		return nil, err
	}

	return &envoytlsv3.UpstreamTlsContext{
		CommonTlsContext: common,
		Sni:              sni,
	}, nil
}

// ResolveCommonSslConfigFromCA creates a CommonTlsContext from a CA certificate string.
func ResolveCommonSslConfigFromCA(caCert string, validation *envoytlsv3.CertificateValidationContext) (*envoytlsv3.CommonTlsContext, error) {
	caCrtData := InlineStringDataSource(caCert)

	tlsContext := &envoytlsv3.CommonTlsContext{
		// default params
		TlsParams: &envoytlsv3.TlsParameters{},
	}
	validation.TrustedCa = caCrtData
	validationCtx := &envoytlsv3.CommonTlsContext_ValidationContext{
		ValidationContext: validation,
	}

	tlsContext.ValidationContextType = validationCtx
	return tlsContext, nil
}

// CleanedSslKeyPair validates and cleans a certificate and key pair.
func CleanedSslKeyPair(certChain, privateKey string) (cleanedChain string, err error) {
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

// InlineStringDataSource returns an Envoy data source that uses the given string as an inline data source.
func InlineStringDataSource(s string) *envoycorev3.DataSource {
	return &envoycorev3.DataSource{
		Specifier: &envoycorev3.DataSource_InlineString{
			InlineString: s,
		},
	}
}

// FileDataSource returns an Envoy data source that uses the given string as a file path.
func FileDataSource(s string) *envoycorev3.DataSource {
	return &envoycorev3.DataSource{
		Specifier: &envoycorev3.DataSource_Filename{
			Filename: s,
		},
	}
}
