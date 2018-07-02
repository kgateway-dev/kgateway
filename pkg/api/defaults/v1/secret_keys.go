package v1

// this file contains expected keys for the certificates in a secret.Data (a map[string]string)
const (
	SslCertificateChainKey           = "tls.crt"
	SslPrivateKeyKey                 = "tls.key"
	SslRootCaKey                     = "tls.root"
)