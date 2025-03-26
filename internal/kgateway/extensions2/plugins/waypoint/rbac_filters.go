package waypoint

import (
	"log"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/waypoint/waypointquery"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"google.golang.org/protobuf/types/known/anypb"
)

// applyHTTPRBACFilters applies RBAC filters to an HTTP filter chain
func applyHTTPRBACFilters(httpChain *ir.HttpFilterChainIR, httpRBAC []*ir.CustomEnvoyFilter, svc waypointquery.Service) {
	log.Printf("Processing HTTP filter chain for service %s/%s", svc.GetNamespace(), svc.GetName())
	log.Printf("HTTP filter chain has %d network filters, %d HTTP filters",
		len(httpChain.CustomNetworkFilters), len(httpChain.CustomHTTPFilters))

	// Apply RBAC filters regardless of the presence of proxy_protocol_authority
	if len(httpRBAC) > 0 {
		log.Printf("Adding %d HTTP RBAC filters", len(httpRBAC))

		// Initialize CustomHTTPFilters if it's nil
		if httpChain.CustomHTTPFilters == nil {
			httpChain.CustomHTTPFilters = []ir.CustomEnvoyFilter{}
		}

		// Log the existing filter chain
		log.Printf("HTTP filter chain before update:")
		for j, f := range httpChain.CustomNetworkFilters {
			log.Printf("  Network filter %d: name=%s", j, f.Name)
		}
		for j, f := range httpChain.CustomHTTPFilters {
			log.Printf("  HTTP filter %d: name=%s", j, f.Name)
		}

		// Add RBAC filters to CustomHTTPFilters
		for _, f := range httpRBAC {
			log.Printf("Adding HTTP RBAC filter: name=%s, stage=%v, weight=%d",
				f.Name, f.FilterStage.RelativeTo, f.FilterStage.Weight)
			httpChain.CustomHTTPFilters = append(httpChain.CustomHTTPFilters, *f)
		}

		// Log the updated filter chain
		log.Printf("HTTP filter chain after update:")
		for j, f := range httpChain.CustomNetworkFilters {
			log.Printf("  Network filter %d: name=%s", j, f.Name)
		}
		for j, f := range httpChain.CustomHTTPFilters {
			log.Printf("  HTTP filter %d: name=%s", j, f.Name)
		}
	} else {
		log.Printf("No HTTP RBAC filters to add")
	}
}

// applyTCPRBACFilters applies RBAC filters to a TCP filter chain
func applyTCPRBACFilters(tcpChain *ir.TcpIR, tcpRBAC []*ir.CustomEnvoyFilter, svc waypointquery.Service) {
	log.Printf("Processing TCP filter chain for service %s/%s", svc.GetNamespace(), svc.GetName())
	log.Printf("TCP filter chain has %d network filters", len(tcpChain.CustomNetworkFilters))

	// Apply RBAC filters regardless of the presence of proxy_protocol_authority
	if len(tcpRBAC) > 0 {
		log.Printf("Adding %d TCP RBAC filters", len(tcpRBAC))

		// Initialize NetworkFilters if it's nil - use appropriate type
		// Note: Since ir.NetworkFilter is undefined, we'll check the actual type
		if tcpChain.NetworkFilters == nil {
			// Use empty slice for the appropriate type
			// You may need to adjust this based on the actual type
			tcpChain.NetworkFilters = []*anypb.Any{}
		}

		// Log the existing filter chain
		log.Printf("TCP filter chain before update:")
		for j, f := range tcpChain.CustomNetworkFilters {
			log.Printf("  Network filter %d: name=%s", j, f.Name)
		}
		for j, f := range tcpChain.NetworkFilters {
			if f != nil {
				log.Printf("  Built-in network filter %d: type=%T", j, f)
			}
		}

		// Add RBAC filters as built-in network filters
		for _, f := range tcpRBAC {
			log.Printf("Adding TCP RBAC filter: name=%s, stage=%v, weight=%d",
				f.Name, f.FilterStage.RelativeTo, f.FilterStage.Weight)
			tcpChain.NetworkFilters = append(tcpChain.NetworkFilters, f.Config)
		}

		// Log the updated filter chain
		log.Printf("TCP filter chain after update:")
		for j, f := range tcpChain.CustomNetworkFilters {
			log.Printf("  Network filter %d: name=%s", j, f.Name)
		}
		for j, f := range tcpChain.NetworkFilters {
			if f != nil {
				log.Printf("  Built-in network filter %d: type=%T", j, f)
			}
		}
	} else {
		log.Printf("No TCP RBAC filters to add")
	}
}
