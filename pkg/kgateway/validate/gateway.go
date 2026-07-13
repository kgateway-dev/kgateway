package validate

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	MetricsPort    gwv1.PortNumber = 9091
	ReadinessPort  gwv1.PortNumber = 8082
	EnvoyAdminPort gwv1.PortNumber = 19000
)

var ErrListenerPortReserved = fmt.Errorf("port is reserved")

// staticReservedPorts are always reserved regardless of gateway configuration.
var staticReservedPorts = sets.New(
	MetricsPort,
	ReadinessPort,
	EnvoyAdminPort,
)

// ListenerPort validates that the given listener port does not conflict with reserved ports.
func ListenerPort(listener ir.Listener, port gwv1.PortNumber) error {
	if staticReservedPorts.Has(port) {
		return fmt.Errorf("invalid port %d in listener: %w", port, ErrListenerPortReserved)
	}
	return nil
}
