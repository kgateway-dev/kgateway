package validate

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

var ErrListenerPortReserved = fmt.Errorf("port is reserved")

var reservedPorts = sets.New[int32](
	9091,  // Metrics port
	8082,  // Readiness port
	19000, // Envoy admin port
)

var agentGatewayReservedPorts = sets.New[int32](
	15020, // Metrics port
	15021, // Readiness port
	15000, // Envoy admin port
)

// ListenerPort validates that the given listener port does not conflict with reserved ports.
func ListenerPort(listener ir.Listener, port gwv1.PortNumber) error {
	return ListenerPortForParent(port, false)
}

func ListenerPortForParent(port int32, agentgateway bool) error {
	if agentgateway {
		if agentGatewayReservedPorts.Has(port) {
			return fmt.Errorf("invalid port %d in listener: %w",
				port, ErrListenerPortReserved)
		}
	} else {
		if reservedPorts.Has(port) {
			return fmt.Errorf("invalid port %d in listener: %w",
				port, ErrListenerPortReserved)
		}
	}
	return nil
}
