package serviceentry

import (
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils/backendaddress"
)

// ServiceAddresses returns the addresses of a Kubernetes Service.
//
// Deprecated: the helper lives in pkg/kgateway/utils/backendaddress so the
// kubernetes and waypoint plugins can share it without importing this plugin.
// This wrapper is kept for existing importers.
func ServiceAddresses(svc *corev1.Service) []string {
	return backendaddress.ServiceAddresses(svc)
}

// ServiceEntryAddresses returns the addresses of a ServiceEntry, including
// auto-allocated ones from status.
//
// Deprecated: see ServiceAddresses; use pkg/kgateway/utils/backendaddress.
func ServiceEntryAddresses(se *networkingclient.ServiceEntry) []string {
	return backendaddress.ServiceEntryAddresses(se)
}
