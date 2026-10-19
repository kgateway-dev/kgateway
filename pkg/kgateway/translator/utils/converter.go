package utils

import (
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// ToListener converts a ListenerSet entry to a Gateway listener without deep-copying it. The result
// aliases the entry's Hostname, TLS and AllowedRoutes, so callers must treat them as read-only,
// as they already do for listeners taken from Gateway.Spec.Listeners.
func ToListener(listenerEntry gwv1.ListenerEntry) gwv1.Listener {
	return gwv1.Listener(listenerEntry)
}

// ToListenerSlice converts ListenerSet entries with ToListener; the results alias the entries.
func ToListenerSlice(listenerEntries []gwv1.ListenerEntry) []gwv1.Listener {
	listeners := make([]gwv1.Listener, len(listenerEntries))
	for i, l := range listenerEntries {
		listeners[i] = ToListener(l)
	}
	return listeners
}
