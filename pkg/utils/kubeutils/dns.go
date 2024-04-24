package kubeutils

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ClusterInternalDomainName is the internal domain for a Kubernetes Cluster
	ClusterInternalDomainName = "cluster.local"
)

// ServiceFQDN returns the FQDN for the Service, assuming it is being accessed from within the Cluster
// https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#services
func ServiceFQDN(serviceMeta metav1.ObjectMeta) string {
	return fmt.Sprintf("%s.%s.svc.%s", serviceMeta.GetName(), serviceMeta.GetNamespace(), ClusterInternalDomainName)
}
