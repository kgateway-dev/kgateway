package certgen

import (
	"crypto/x509"
	"fmt"

	"github.com/solo-io/go-utils/certutils"
	"k8s.io/client-go/util/cert"
)

func GenCerts(svcName, svcNamespace string) (*certutils.Certificates, error) {
	return certutils.GenerateSelfSignedCertificate(cert.Config{
		CommonName:   fmt.Sprintf("%s.%s.svc", svcName, svcNamespace),
		Organization: []string{"solo.io"},
		AltNames: cert.AltNames{
			DNSNames: []string{
				svcName,
				fmt.Sprintf("%s.%s", svcName, svcNamespace),
				fmt.Sprintf("%s.%s.svc", svcName, svcNamespace),
				fmt.Sprintf("%s.%s.svc.cluster.local", svcName, svcNamespace),
			},
		},
		Usages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
}
