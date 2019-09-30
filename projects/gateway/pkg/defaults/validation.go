package defaults

import (
	"fmt"
	"path/filepath"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	v1 "k8s.io/api/core/v1"
)

var (
	GlooProxyValidationServerAddr = fmt.Sprintf("gloo:%v", defaults.GlooValidationPort)
	ValidationWebhookBindPort     = 8443
	ValidationWebhookTlsCertPath  = filepath.Join("/etc", "gateway", "validation-certs", v1.TLSCertKey)
	ValidationWebhookTlsKeyPath   = filepath.Join("/etc", "gateway", "validation-certs", v1.TLSPrivateKeyKey)
)
