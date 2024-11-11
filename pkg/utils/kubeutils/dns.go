package kubeutils

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/network"
)

// ServiceFQDN returns the FQDN for the Service, assuming it is being accessed from within the Cluster
// https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#services
func ServiceFQDN(serviceMeta metav1.ObjectMeta) string {
	return network.GetServiceHostname(serviceMeta.Name, serviceMeta.Namespace)
}
