package proxy_syncer

import (
	"slices"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/validate"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// bootstrapCoverage records the bootstrap settings that one Gateway translation was
// validated with: the encoded validation subset of the Gateway's current bootstrap,
// and the reserved ports, which no Gateway listener may use because a bootstrap
// listens on them. A proxy accepts the translation's configuration when its
// bootstrap has the same subset and listens only on reserved ports.
type bootstrapCoverage struct {
	// subset is the encoded subset; empty means Envoy's defaults.
	subset string
	// ports are the reserved ports, sorted; nil means the default reserved ports.
	ports []uint32
}

// newBootstrapCoverage returns the coverage of a translation that used vb, the
// Gateway's validation bootstraps. A nil vb, as for every Gateway without
// bootstrap overlays, gives the zero value: Envoy's defaults and the default
// reserved ports.
func newBootstrapCoverage(vb *ir.GatewayValidationBootstraps) bootstrapCoverage {
	if vb == nil {
		return bootstrapCoverage{}
	}

	// The reserved ports are the ones validate.ListenerPortForBootstrap rejects.
	return bootstrapCoverage{subset: vb.Subset.Subset, ports: validate.ReservedPorts(vb)}
}

// covers reports whether the translation was validated with the settings of the
// bootstrap b. It returns true for a nil b, which is a client without a pod.
func (c bootstrapCoverage) covers(b *ir.PodBootstrap) bool {
	// A client without a pod is not checked.
	if b == nil {
		return true
	}

	// The bootstrap's subset must be the one the translation was validated with.
	if b.Subset != c.subset {
		return false
	}

	// Every port the bootstrap listens on must be reserved.
	reserved := c.ports
	if reserved == nil {
		reserved = validate.DefaultReservedPorts()
	}
	for _, p := range b.Ports {
		if !slices.Contains(reserved, p) {
			return false
		}
	}
	return true
}

// Equals compares the subset and the ports.
func (c bootstrapCoverage) Equals(in bootstrapCoverage) bool {
	return c.subset == in.subset && slices.Equal(c.ports, in.ports)
}
