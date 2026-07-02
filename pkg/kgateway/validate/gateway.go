package validate

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

var ErrListenerPortReserved = fmt.Errorf("port is reserved")

// MetricsPort is the port used by the Envoy proxy stats/metrics server. It is
// only occupied when the proxy stats server is enabled (the default), which is
// configured per-Gateway via GatewayParameters.
const MetricsPort int32 = 9091

var reservedPorts = sets.New[int32](
	8082,  // Readiness port
	19000, // Envoy admin port
)

// ListenerPort validates that the given listener port does not conflict with reserved ports.
// The metrics port (9091) is only reserved when the proxy stats server is enabled; pass
// statsDisabled=true (derived from the Gateway's GatewayParameters) to allow it.
func ListenerPort(listener ir.Listener, port gwv1.PortNumber, statsDisabled bool) error {
	if !statsDisabled && int32(port) == MetricsPort {
		return fmt.Errorf("invalid port %d in listener: %w",
			port, ErrListenerPortReserved)
	}
	if reservedPorts.Has(port) {
		return fmt.Errorf("invalid port %d in listener: %w",
			port, ErrListenerPortReserved)
	}
	return nil
}
