package validate

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	MetricsPort    int32 = 9091
	ReadinessPort  int32 = 8082
	EnvoyAdminPort int32 = 19000
)

var ErrListenerPortReserved = fmt.Errorf("port is reserved")

var reservedPorts = sets.New(
	MetricsPort,
	ReadinessPort,
	EnvoyAdminPort,
)

// ListenerPort validates that the given listener port does not conflict with reserved ports.
func ListenerPort(gwp *kgateway.GatewayParameters, listener ir.Listener, port gwv1.PortNumber) error {
	if port == MetricsPort {
		if gwp != nil && gwp.Spec.Kube != nil && gwp.Spec.Kube.Stats != nil && gwp.Spec.Kube.Stats.Enabled != nil && !*gwp.Spec.Kube.Stats.Enabled {
			return nil
		}
	}

	if reservedPorts.Has(port) {
		return fmt.Errorf("invalid port %d in listener: %w",
			port, ErrListenerPortReserved)
	}
	return nil
}
