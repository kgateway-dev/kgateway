package certgen

import (
	"crypto/x509"
	"fmt"

	"github.com/solo-io/k8s-utils/certutils"
	"k8s.io/client-go/util/cert"
	"knative.dev/pkg/network"
)

func GenCerts(svcName, svcNamespace string) (*certutils.Certificates, error) {
	return certutils.GenerateSelfSignedCertificate(cert.Config{
		CommonName:   fmt.Sprintf("%s.%s.svc", svcName, svcNamespace),
		Organization: []string{"solo.io"},
		AltNames: cert.AltNames{
			DNSNames: getDnsNames(svcName, svcNamespace),
		},
		Usages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	})
}

// Returns true if any of the given DNS names matches any of the DNS names generated for the given service.
func ValidForService(dnsNames []string, svcName, svcNamespace string) bool {
	svcDnsNames := getDnsNames(svcName, svcNamespace)
	for _, name := range dnsNames {
		for _, svcDnsName := range svcDnsNames {
			if name == svcDnsName {
				return true
			}
		}
	}
	return false
}

func getDnsNames(svcName, svcNamespace string) []string {
	return []string{
		svcName,
		fmt.Sprintf("%s.%s", svcName, svcNamespace),
		fmt.Sprintf("%s.%s.svc", svcName, svcNamespace),
		fmt.Sprintf("%s.%s.svc.%s", svcName, svcNamespace, network.GetClusterDomainName()),
	}
}
