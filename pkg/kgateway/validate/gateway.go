package validate

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	// DefaultMetricsPort is the default port for the Envoy stats/metrics server.
	DefaultMetricsPort int32 = 9091
	ReadinessPort      int32 = 8082
	EnvoyAdminPort     int32 = 19000
)

var ErrListenerPortReserved = fmt.Errorf("port is reserved")

// staticReservedPorts are always reserved regardless of gateway configuration.
var staticReservedPorts = sets.New(
	ReadinessPort,
	EnvoyAdminPort,
)

// ListenerPort validates that the given listener port does not conflict with reserved ports.
func ListenerPort(gwp *kgateway.GatewayParameters, listener ir.Listener, port gwv1.PortNumber) error {
	statsPort := DefaultMetricsPort
	if gwp != nil {
		if p := gwp.Spec.GetKube().GetStats().GetPort(); p != nil {
			statsPort = *p
		}
	}

	if port == statsPort {
		statsEnabled := gwp == nil || ptr.Deref(gwp.Spec.GetKube().GetStats().GetEnabled(), true)
		if statsEnabled {
			return fmt.Errorf("invalid port %d in listener: %w", port, ErrListenerPortReserved)
		}
	}

	if staticReservedPorts.Has(port) {
		return fmt.Errorf("invalid port %d in listener: %w", port, ErrListenerPortReserved)
	}
	return nil
}
