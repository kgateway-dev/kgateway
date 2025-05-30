package utils

import (
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"
)

func ToListener(listenerEntry gwxv1a1.ListenerEntry) gwv1.Listener {
	duplicate := listenerEntry.DeepCopy()
	return gwv1.Listener{
		Name:          duplicate.Name,
		Hostname:      duplicate.Hostname,
		Port:          duplicate.Port,
		Protocol:      duplicate.Protocol,
		TLS:           duplicate.TLS,
		AllowedRoutes: duplicate.AllowedRoutes,
	}
}

func ToListenerSlice(listenerEntries []gwxv1a1.ListenerEntry) []gwv1.Listener {
	listeners := make([]gwv1.Listener, len(listenerEntries))
	for i, l := range listenerEntries {
		listeners[i] = ToListener(l)
	}
	return listeners
}
