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

// ListenerPort validates that the given listener port does not conflict with reserved ports.
// When disableStatsOnProxy is true, port 9091 (the metrics port) is not considered reserved.
func ListenerPort(listener ir.Listener, port gwv1.PortNumber, disableStatsOnProxy bool) error {
	if disableStatsOnProxy && port == 9091 {
		return nil
	}
	if reservedPorts.Has(port) {
		return fmt.Errorf("invalid port %d in listener: %w",
			port, ErrListenerPortReserved)
	}
	return nil
}
