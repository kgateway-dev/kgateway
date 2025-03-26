package waypoint

import (
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/waypoint/waypointquery"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"google.golang.org/protobuf/types/known/anypb"
)

// applyHTTPRBACFilters applies RBAC filters to an HTTP filter chain
func applyHTTPRBACFilters(httpChain *ir.HttpFilterChainIR, httpRBAC []*ir.CustomEnvoyFilter, svc waypointquery.Service) {
	// Apply RBAC filters regardless of the presence of proxy_protocol_authority
	if len(httpRBAC) > 0 {
		// Initialize CustomHTTPFilters if it's nil
		if httpChain.CustomHTTPFilters == nil {
			httpChain.CustomHTTPFilters = []ir.CustomEnvoyFilter{}
		}

		// Add RBAC filters to CustomHTTPFilters
		for _, f := range httpRBAC {
			httpChain.CustomHTTPFilters = append(httpChain.CustomHTTPFilters, *f)
		}
	}
}

// applyTCPRBACFilters applies RBAC filters to a TCP filter chain
func applyTCPRBACFilters(tcpChain *ir.TcpIR, tcpRBAC []*ir.CustomEnvoyFilter, svc waypointquery.Service) {
	// Apply RBAC filters regardless of the presence of proxy_protocol_authority
	if len(tcpRBAC) > 0 {
		if tcpChain.NetworkFilters == nil {
			tcpChain.NetworkFilters = []*anypb.Any{}
		}

		// Add RBAC filters as built-in network filters
		for _, f := range tcpRBAC {
			tcpChain.NetworkFilters = append(tcpChain.NetworkFilters, f.Config)
		}
	}
}
