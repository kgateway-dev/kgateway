package validate

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

var ErrListenerPortReserved = fmt.Errorf("port is reserved")

var reservedPorts = sets.New[int32](
	9091,  // Metrics port
	8082,  // Readiness port
	19000, // Envoy admin port
)

// Gateway validates the given Gateway IR. Currently, it validates the listener ports for
// conflicts with reserved ports, and should be extended to include other validations that are
// common to the Deployer and Translator.
func Gateway(gw *ir.Gateway) error {
	if gw == nil {
		return nil
	}

	// Validate listener ports for conflicts
	for _, l := range gw.Listeners {
		if err := ListenerPort(l, l.Port); err != nil {
			return err
		}
	}

	return nil
}

// ListenerPort validates that the given listener port does not conflict with reserved ports.
func ListenerPort(listener ir.Listener, port gwv1.PortNumber) error {
	if reservedPorts.Has(int32(port)) {
		return fmt.Errorf("invalid port %d in listener %s/%s/%s: %w",
			port, getListenerSourceKind(listener.Parent), kubeutils.NamespacedNameFrom(listener.Parent), listener.Name, ErrListenerPortReserved)
	}
	return nil
}

func getListenerSourceKind(obj client.Object) string {
	switch obj.(type) {
	case *gwv1.Gateway:
		return wellknown.GatewayKind
	case *gwxv1a1.XListenerSet:
		return wellknown.XListenerSetKind
	default:
		return ""
	}
}
