package waypoint

import (
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/waypoint/waypointquery"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

// applyHTTPRBACFilters applies RBAC filters to an HTTP filter chain
func applyHTTPRBACFilters(httpChain *ir.HttpFilterChainIR, httpRBAC []*ir.CustomEnvoyFilter, svc waypointquery.Service) {
	// Apply RBAC filters regardless of the presence of proxy_protocol_authority
	// Initialize CustomHTTPFilters if it's nil
	if len(httpRBAC) > 0 {

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
		if tcpChain.FilterChainCommon.CustomNetworkFilters == nil {
			tcpChain.FilterChainCommon.CustomNetworkFilters = []ir.CustomEnvoyFilter{}
		}

		// Add RBAC filters as built-in network filters
		for _, f := range tcpRBAC {
			tcpChain.FilterChainCommon.CustomNetworkFilters = append(tcpChain.FilterChainCommon.CustomNetworkFilters, ir.CustomEnvoyFilter{
				Name: f.Name,
				Config: f.Config,
				FilterStage: f.FilterStage,
			})
		}
	}
}
