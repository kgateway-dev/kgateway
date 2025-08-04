package kubeutils

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceFQDN returns the FQDN for the Service, assuming it is being accessed from within the Cluster
func ServiceFQDN(serviceMeta metav1.ObjectMeta) string {
	return fmt.Sprintf("%s.%s.svc.%s", serviceMeta.Name, serviceMeta.Namespace, GetClusterDomainName())
}
