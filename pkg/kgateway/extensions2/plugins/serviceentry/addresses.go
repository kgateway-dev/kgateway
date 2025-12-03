package serviceentry

import (
	networkingv1beta1 "istio.io/api/networking/v1beta1"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/slices"
	corev1 "k8s.io/api/core/v1"
)

// ServiceAddresses returns the addresses of a Kubernetes Service.
// ClusterIPs are optional in a Service and if exists will include the address of ClusterIP.
// Value can also be "None" (headless service) in both ClusterIPs and ClusterIP, which are excluded.
func ServiceAddresses(svc *corev1.Service) []string {
	var addrs []string
	if len(svc.Spec.ClusterIPs) > 0 {
		for _, ip := range svc.Spec.ClusterIPs {
			if ip != "" && ip != "None" {
				addrs = append(addrs, ip)
			}
		}
	}
	if len(addrs) == 0 && len(svc.Spec.ClusterIP) > 0 && svc.Spec.ClusterIP != "None" {
		addrs = []string{svc.Spec.ClusterIP}
	}
	return addrs
}

// ServiceEntryAddresses returns the addresses of a ServiceEntry.
// This includes both manually specified addresses (Spec.Addresses) and auto-allocated addresses (Status.Addresses).
// Auto-allocated addresses are particularly important for ServiceEntries with ISTIO_META_DNS_AUTO_ALLOCATE enabled.
func ServiceEntryAddresses(se *networkingclient.ServiceEntry) []string {
	// Combine spec addresses with status addresses (which include auto-allocated IPs)
	addrs := append(se.Spec.GetAddresses(), slices.Map(se.Status.GetAddresses(), func(a *networkingv1beta1.ServiceEntryAddress) string {
		return a.Value
	})...)
	return addrs
}
