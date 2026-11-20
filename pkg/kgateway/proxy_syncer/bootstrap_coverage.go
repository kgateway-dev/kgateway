package proxy_syncer

import (
	"slices"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/validate"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// bootstrapCoverage is what one Gateway translation was validated under: the
// encoded validation subsets, and the ports it gives no Gateway listener because
// a bootstrap listens on them. A proxy whose bootstrap's subset is among the
// subsets, and whose bootstrap's ports are all reserved, accepts the
// translation's configuration.
type bootstrapCoverage struct {
	// subsets are the encoded subsets, sorted; nil means Envoy's defaults only.
	subsets []string
	// ports are the reserved ports, sorted; nil means the default reserved ports.
	ports []uint32
}

// newBootstrapCoverage returns the coverage of a translation that used vb, the
// Gateway's validation bootstraps. A nil vb, as for every Gateway without
// bootstrap patches, gives the zero value: Envoy's defaults and the default
// reserved ports.
func newBootstrapCoverage(vb *ir.GatewayValidationBootstraps) bootstrapCoverage {
	// Without validation bootstraps, only the defaults were validated.
	if vb == nil {
		return bootstrapCoverage{}
	}

	// Collect the subsets the translation validated under.
	var subsets []string
	for _, s := range vb.Subsets {
		subsets = append(subsets, s.Subset)
	}
	slices.Sort(subsets)

	// Reserve the current bootstrap's ports, or the default ones when it lists
	// none, and the ports running pods' bootstraps listen on, as
	// validate.ListenerPortForBootstrap does.
	ports := slices.Clone(vb.ReservedPorts)
	if len(ports) == 0 {
		ports = validate.DefaultReservedPorts()
	}
	for _, p := range vb.PodReservedPorts {
		ports = append(ports, p.Port)
	}
	slices.Sort(ports)
	return bootstrapCoverage{subsets: slices.Compact(subsets), ports: slices.Compact(ports)}
}

// covers reports whether the translation was validated under b. A nil b, a
// client without a pod, is covered.
func (c bootstrapCoverage) covers(b *ir.PodBootstrap) bool {
	// A client without a pod is not checked.
	if b == nil {
		return true
	}

	// The bootstrap's subset must be one the translation validated under.
	if c.subsets == nil {
		if b.Subset != "" {
			return false
		}
	} else if _, found := slices.BinarySearch(c.subsets, b.Subset); !found {
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

// Equals compares the subsets and the ports.
func (c bootstrapCoverage) Equals(in bootstrapCoverage) bool {
	return slices.Equal(c.subsets, in.subsets) && slices.Equal(c.ports, in.ports)
}
