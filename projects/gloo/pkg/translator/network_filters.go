package translator

import (
	"sort"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	validationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
)

type NetworkFilterTranslator interface {
	ComputeNetworkFilters(params plugins.Params) []*envoy_config_listener_v3.Filter
}

var _ NetworkFilterTranslator = new(httpNetworkFilterTranslator)

type httpNetworkFilterTranslator struct {
	// List of HttpFilterPlugins to process
	plugins []plugins.HttpFilterPlugin
	// A Gloo HttpListener produces a single filter chain, with its own set of NetworkFilters
	listener *v1.HttpListener
	// The report where warnings/errors are persisted
	report *validationapi.HttpListenerReport
	// The name of the RouteConfiguration for the HttpConnectionManager
	routeConfigName string
}

func (h *httpNetworkFilterTranslator) ComputeNetworkFilters(params plugins.Params) []*envoy_config_listener_v3.Filter {
	// return if listener has no virtual hosts
	if len(h.listener.GetVirtualHosts()) == 0 {
		return nil
	}

	var networkFilters []plugins.StagedNetworkFilter

	// We used to support a ListenerFilterPlugin interface, which was used to generate
	// a list of NetworkFilters. That plugin wasn't implemented in the codebase so it
	// was removed. If we want to support other network filters, we would process
	// those plugins here.

	// Check that we don't refer to nonexistent auth config
	// TODO (sam-heilbron)
	// This is a partial duplicate of the open source ExtauthTranslatorSyncer
	// We should find a single place to define this configuration
	for i, vHost := range h.listener.GetVirtualHosts() {
		acRef := vHost.GetOptions().GetExtauth().GetConfigRef()
		if acRef != nil {
			if _, err := params.Snapshot.AuthConfigs.Find(acRef.GetNamespace(), acRef.GetName()); err != nil {
				validation.AppendVirtualHostError(
					h.report.GetVirtualHostReports()[i],
					validationapi.VirtualHostReport_Error_ProcessingError,
					"auth config not found: "+acRef.String())
			}
		}
	}

	// add the http connection manager filter after all the InAuth Listener Filters
	httpConnectionManagerFilter := h.computeHttpConnectionManagerFilter(params)
	networkFilters = append(networkFilters, plugins.StagedNetworkFilter{
		NetworkFilter: httpConnectionManagerFilter,
		Stage:         plugins.AfterStage(plugins.AuthZStage),
	})

	return sortNetworkFilters(networkFilters)
}

func sortNetworkFilters(filters plugins.StagedNetworkFilterList) []*envoy_config_listener_v3.Filter {
	sort.Sort(filters)
	var sortedFilters []*envoy_config_listener_v3.Filter
	for _, filter := range filters {
		sortedFilters = append(sortedFilters, filter.NetworkFilter)
	}
	return sortedFilters
}
