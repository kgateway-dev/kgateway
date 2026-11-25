package validate

import (
	"errors"
	"fmt"
	"slices"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

var ErrListenerPortReserved = errors.New("port is reserved")

// DefaultReservedPorts returns the ports the generated bootstrap listens on, sorted.
// They are reserved for every Gateway, including one whose bootstrap overlay moves a
// static listener off them: proxy pods started before the overlay, or without the
// kgateway.dev/xds-validation-bootstrap annotation, still listen on them.
func DefaultReservedPorts() []uint32 {
	return []uint32{wellknown.EnvoyReadinessPort, wellknown.EnvoyMetricsPort, wellknown.EnvoyAdminPort}
}

// ListenerPort validates that the given listener port does not conflict with reserved ports.
func ListenerPort(listener ir.Listener, port gwv1.PortNumber) error {
	return ListenerPortForBootstrap(port, nil)
}

// ReservedPorts returns every port no Gateway listener may use: the default reserved
// ports and the ports the Gateway's current bootstrap listens on, vb's ReservedPorts,
// sorted without duplicates. A nil vb means the default reserved ports only.
func ReservedPorts(vb *ir.GatewayValidationBootstraps) []uint32 {
	ports := slices.Concat(DefaultReservedPorts(), vb.GetReservedPorts())
	slices.Sort(ports)
	return slices.Compact(ports)
}

// ListenerPortForBootstrap validates that port is not one of ReservedPorts. A nil vb
// means the default reserved ports only.
func ListenerPortForBootstrap(port gwv1.PortNumber, vb *ir.GatewayValidationBootstraps) error {
	p := uint32(port) //nolint:gosec // G115: listener ports are positive

	// A port the merged bootstrap listens on says so, since the overlay is what to change.
	if slices.Contains(vb.GetReservedPorts(), p) && !slices.Contains(DefaultReservedPorts(), p) {
		return fmt.Errorf("invalid port %d in listener: the proxy bootstrap, with the GatewayParameters bootstrap overlay applied, listens on it: %w", port, ErrListenerPortReserved)
	}

	// The generated bootstrap's ports are always reserved.
	if slices.Contains(ReservedPorts(vb), p) {
		return fmt.Errorf("invalid port %d in listener: %w", port, ErrListenerPortReserved)
	}
	return nil
}
