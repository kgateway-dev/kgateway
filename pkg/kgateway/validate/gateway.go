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
// They are reserved for a Gateway whose bootstrap has no patches, and while a proxy pod
// that started without the kgateway.dev/xds-validation-bootstrap annotation runs.
func DefaultReservedPorts() []uint32 {
	return []uint32{wellknown.EnvoyReadinessPort, wellknown.EnvoyMetricsPort, wellknown.EnvoyAdminPort}
}

// ListenerPort validates that the given listener port does not conflict with reserved ports.
func ListenerPort(listener ir.Listener, port gwv1.PortNumber) error {
	return ListenerPortForBootstrap(port, nil)
}

// ListenerPortForBootstrap validates that port is not one that a bootstrap of the
// Gateway's proxies listens on: the current bootstrap, whose ports are vb's
// ReservedPorts or, when those are empty, the default reserved ports; or the earlier
// bootstrap of a pod that still runs, vb's PodReservedPorts. A nil vb means the default
// reserved ports only.
func ListenerPortForBootstrap(port gwv1.PortNumber, vb *ir.GatewayValidationBootstraps) error {
	p := uint32(port) //nolint:gosec // G115: listener ports are positive

	// Without a bootstrap with patches, the generated bootstrap's ports are reserved.
	current := vb.GetReservedPorts()
	if len(current) == 0 {
		if slices.Contains(DefaultReservedPorts(), p) {
			return fmt.Errorf("invalid port %d in listener: %w", port, ErrListenerPortReserved)
		}
	} else if slices.Contains(current, p) {
		return fmt.Errorf("invalid port %d in listener: the proxy bootstrap, with the GatewayParameters bootstrap patch applied, listens on it: %w", port, ErrListenerPortReserved)
	}

	// Every pod receives the same listeners, so a pod still running an earlier bootstrap
	// that listens on the port would reject the listener as a duplicate address.
	i := slices.IndexFunc(vb.GetPodReservedPorts(), func(r ir.PodReservedPort) bool { return r.Port == p })
	if i >= 0 {
		return fmt.Errorf("invalid port %d in listener: proxy pod %s still runs an earlier bootstrap that listens on it; the port is free once the rollout completes: %w",
			port, vb.GetPodReservedPorts()[i].Pod, ErrListenerPortReserved)
	}
	return nil
}
