package validate

import (
	"errors"
	"fmt"
	"slices"

	"k8s.io/apimachinery/pkg/util/sets"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

var ErrListenerPortReserved = errors.New("port is reserved")

// staticReservedPorts are the ports the generated bootstrap listens on. They are
// reserved for every Gateway whose bootstrap has no patches.
var staticReservedPorts = sets.New(
	gwv1.PortNumber(wellknown.EnvoyMetricsPort),   //nolint:gosec // G115: the default ports fit in int32
	gwv1.PortNumber(wellknown.EnvoyReadinessPort), //nolint:gosec // G115: the default ports fit in int32
	gwv1.PortNumber(wellknown.EnvoyAdminPort),     //nolint:gosec // G115: the default ports fit in int32
)

// ListenerPort validates that the given listener port does not conflict with reserved ports.
func ListenerPort(listener ir.Listener, port gwv1.PortNumber) error {
	return ListenerPortForBootstrap(port, nil)
}

// ListenerPortForBootstrap validates that port is not one the Gateway's proxy
// bootstrap listens on. bootstrapPorts are those ports, read from a bootstrap with
// patches, and replace the static reserved ports. When bootstrapPorts is empty, the
// static reserved ports apply.
func ListenerPortForBootstrap(port gwv1.PortNumber, bootstrapPorts []uint32) error {
	// Without a bootstrap with patches, the generated bootstrap's ports are reserved.
	if len(bootstrapPorts) == 0 {
		if staticReservedPorts.Has(port) {
			return fmt.Errorf("invalid port %d in listener: %w", port, ErrListenerPortReserved)
		}
		return nil
	}

	// Otherwise exactly the ports the patched bootstrap listens on are reserved.
	if slices.Contains(bootstrapPorts, uint32(port)) { //nolint:gosec // G115: listener ports are positive
		return fmt.Errorf("invalid port %d in listener: the proxy bootstrap, with the GatewayParameters bootstrap patches applied, listens on it: %w", port, ErrListenerPortReserved)
	}
	return nil
}
