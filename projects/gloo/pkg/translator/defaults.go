package translator

import (
	"time"
)

const (
	SslCertificateChainKey = "tls.crt"
	SslPrivateKeyKey       = "tls.key"
	SslRootCaKey           = "tls.root"
)

var (
	ClusterConnectionTimeout = time.Second * 5
)
