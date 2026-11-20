package bootstrappatch

import (
	"slices"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
)

// ListenerPorts returns, sorted and without duplicates, the ports the bootstrap
// listens on: the admin address, and each static listener's address and additional
// addresses. A pipe or internal address has no port and is skipped.
func ListenerPorts(bs *envoybootstrapv3.Bootstrap) []uint32 {
	// Collect the addresses.
	addrs := []*envoycorev3.Address{bs.GetAdmin().GetAddress()}
	for _, l := range bs.GetStaticResources().GetListeners() {
		addrs = append(addrs, l.GetAddress())
		for _, a := range l.GetAdditionalAddresses() {
			addrs = append(addrs, a.GetAddress())
		}
	}

	// Keep each socket address's port.
	var ports []uint32
	for _, a := range addrs {
		if port := a.GetSocketAddress().GetPortValue(); port != 0 {
			ports = append(ports, port)
		}
	}
	slices.Sort(ports)
	return slices.Compact(ports)
}
