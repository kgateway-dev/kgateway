package bootstrappatch

import (
	"fmt"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"k8s.io/apimachinery/pkg/types"
)

// downstreamConnectionsMonitor is the resource monitor the
// envoyContainer.bootstrap.maxDownstreamConnections field generates.
const downstreamConnectionsMonitor = "envoy.resource_monitors.global_downstream_max_connections"

// CuratedOverride is a path an overlay sets that a GatewayParameters field also
// generates. The overlay replaces what the field generates, which can drop a value
// the user did not mean to change.
type CuratedOverride struct {
	// Field is the GatewayParameters field, relative to spec.kube.
	Field string
	// Path is the bootstrap path the field generates and the overlay sets.
	Path string
	// Source is the GatewayParameters whose overlay sets the path.
	Source types.NamespacedName
}

func (o CuratedOverride) String() string {
	return fmt.Sprintf("%s replaces what %s generates (GatewayParameters %s)", o.Path, o.Field, o.Source)
}

// curatedPaths lists the bootstrap paths each GatewayParameters field generates.
// present reports whether a bootstrap has the path.
var curatedPaths = []struct {
	field   string
	path    string
	present func(*envoybootstrapv3.Bootstrap) bool
}{
	{
		field:   "envoyContainer.bootstrap.dnsResolver",
		path:    "typed_dns_resolver_config",
		present: func(bs *envoybootstrapv3.Bootstrap) bool { return bs.GetTypedDnsResolverConfig() != nil },
	},
	{
		field: "envoyContainer.bootstrap.maxDownstreamConnections",
		path:  "overload_manager.resource_monitors[name=" + downstreamConnectionsMonitor + "]",
		present: func(bs *envoybootstrapv3.Bootstrap) bool {
			return containsName(bs.GetOverloadManager().GetResourceMonitors(), downstreamConnectionsMonitor)
		},
	},
	{
		field:   "stats.matcher",
		path:    "stats_config.stats_matcher",
		present: func(bs *envoybootstrapv3.Bootstrap) bool { return bs.GetStatsConfig().GetStatsMatcher() != nil },
	},
}

// curatedOverrides returns each path overlay sets that a GatewayParameters field
// generated in base.
func curatedOverrides(base, overlay *envoybootstrapv3.Bootstrap, source types.NamespacedName) []CuratedOverride {
	var out []CuratedOverride
	for _, c := range curatedPaths {
		if c.present(base) && c.present(overlay) {
			out = append(out, CuratedOverride{Field: c.field, Path: c.path, Source: source})
		}
	}
	return out
}
