package validate

import (
	"errors"
	"fmt"
	"slices"

	"k8s.io/apimachinery/pkg/util/sets"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	MetricsPort    gwv1.PortNumber = 9091
	ReadinessPort  gwv1.PortNumber = 8082
	EnvoyAdminPort gwv1.PortNumber = 19000
)

var ErrListenerPortReserved = errors.New("port is reserved")

// staticReservedPorts are always reserved regardless of gateway configuration.
var staticReservedPorts = sets.New(
	MetricsPort,
	ReadinessPort,
	EnvoyAdminPort,
)

// StaticReservedPorts returns, sorted, the ports reserved for every Gateway: those of
// the generated bootstrap's admin address and static listeners.
func StaticReservedPorts() []uint32 {
	out := make([]uint32, 0, staticReservedPorts.Len())
	for p := range staticReservedPorts {
		out = append(out, uint32(p)) //nolint:gosec // G115: the reserved ports are positive constants
	}
	slices.Sort(out)
	return out
}

// ListenerPort validates that the given listener port does not conflict with reserved ports.
func ListenerPort(listener ir.Listener, port gwv1.PortNumber) error {
	return ListenerPortForBootstrap(port, nil)
}

// ListenerPortForBootstrap validates that port is not one the Gateway's proxy
// bootstrap listens on. bootstrapPorts are those ports, read from a bootstrap with
// patches; the static reserved ports apply as well.
func ListenerPortForBootstrap(port gwv1.PortNumber, bootstrapPorts []uint32) error {
	if staticReservedPorts.Has(port) {
		return fmt.Errorf("invalid port %d in listener: %w", port, ErrListenerPortReserved)
	}
	if slices.Contains(bootstrapPorts, uint32(port)) { //nolint:gosec // G115: listener ports are positive
		return fmt.Errorf("invalid port %d in listener: the proxy bootstrap, with the GatewayParameters bootstrap patches applied, listens on it: %w", port, ErrListenerPortReserved)
	}
	return nil
}
