package kubeutils

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Based on knative domain.go: https://github.com/knative/pkg/blob/4c6fea7360fcb6e70551316991c0b7b99dcfb3bd/network/domain.go#L28
const (
	resolverFileName    = "/etc/resolv.conf"
	clusterDomainEnvKey = "CLUSTER_DOMAIN"
	defaultDomainName   = "cluster.local"
)

var (
	domainName = defaultDomainName
	once       sync.Once
)

// ServiceFQDN returns the FQDN for the Service, assuming it is being accessed from within the Cluster
func ServiceFQDN(serviceMeta metav1.ObjectMeta) string {
	return fmt.Sprintf("%s.%s.svc.%s", serviceMeta.Name, serviceMeta.Namespace, GetClusterDomainName())
}

// GetClusterDomainName returns cluster's domain name or an error
// Closes issue: https://github.com/knative/eventing/issues/714
func GetClusterDomainName() string {
	once.Do(func() {
		f, err := os.Open(resolverFileName)
		if err != nil {
			return
		}
		defer f.Close()
		domainName = getClusterDomainName(f)
	})
	return domainName
}

func getClusterDomainName(r io.Reader) string {
	// First look in the conf file.
	for scanner := bufio.NewScanner(r); scanner.Scan(); {
		elements := strings.Split(scanner.Text(), " ")
		if elements[0] != "search" {
			continue
		}
		for _, e := range elements[1:] {
			if strings.HasPrefix(e, "svc.") {
				return strings.TrimSuffix(e[4:], ".")
			}
		}
	}

	// Then look in the ENV.
	if domain := os.Getenv(clusterDomainEnvKey); len(domain) > 0 {
		return domain
	}

	// For all abnormal cases return default domain name.
	return defaultDomainName
}
