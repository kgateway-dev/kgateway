package translator

import (
	"sort"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/wrappers"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/log"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	validationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
)

type NetworkFilterTranslator interface {
	ComputeNetworkFilters(params plugins.Params) ([]*envoy_config_listener_v3.Filter, error)
}

var _ NetworkFilterTranslator = new(httpNetworkFilterTranslator)

const (
	DefaultHttpStatPrefix = "http"
)

type httpNetworkFilterTranslator struct {
	// A Gloo HttpListener produces a single filter chain, with its own set of NetworkFilters
	listener *v1.HttpListener
	// The report where warnings/errors are persisted
	report *validationapi.HttpListenerReport
	// List of NetworkFilterPlugin to process. This is not added to the HCM as that should only process HTTP filters
	networkPlugins []plugins.NetworkFilterPlugin
	// The implementation for generating the HttpConnectionManager NetworkFilter
	hcmNetworkFilterTranslator *hcmNetworkFilterTranslator
}

func NewHttpListenerNetworkFilterTranslator(
	parentListener *v1.Listener,
	listener *v1.HttpListener,
	report *validationapi.HttpListenerReport,
	networkPlugins []plugins.NetworkFilterPlugin,
	httpPlugins []plugins.HttpFilterPlugin,
	hcmPlugins []plugins.HttpConnectionManagerPlugin,
	routeConfigName string,
) *httpNetworkFilterTranslator {
	return &httpNetworkFilterTranslator{
		listener:       listener,
		report:         report,
		networkPlugins: networkPlugins,
		hcmNetworkFilterTranslator: &hcmNetworkFilterTranslator{
			parentListener:  parentListener,
			listener:        listener,
			report:          report,
			httpPlugins:     httpPlugins,
			hcmPlugins:      hcmPlugins,
			routeConfigName: routeConfigName,
		},
	}
}

func (n *httpNetworkFilterTranslator) computePreHCMFilters(params plugins.Params) []plugins.StagedNetworkFilter {
	var networkFilters []plugins.StagedNetworkFilter
	// Process the network filters.
	for _, plug := range n.networkPlugins {
		stagedFilters, err := plug.NetworkFiltersHTTP(params, n.listener)
		if err != nil {
			validation.AppendHTTPListenerError(n.report, validationapi.HttpListenerReport_Error_ProcessingError, err.Error())
		}

		for _, nf := range stagedFilters {
			if nf.NetworkFilter == nil {
				log.Warnf("plugin %v implements NetworkFilters() but returned nil", plug.Name())
				continue
			}
			networkFilters = append(networkFilters, nf)
		}
	}
	return networkFilters
}

func (n *httpNetworkFilterTranslator) ComputeNetworkFilters(params plugins.Params) ([]*envoy_config_listener_v3.Filter, error) {
	if !settingsutil.MaybeFromContext(params.Ctx).GetGateway().GetTranslateEmptyGateways().GetValue() {
		// return if listener has no virtual hosts
		if len(n.listener.GetVirtualHosts()) == 0 {
			return nil, nil
		}
	}

	// We used to support a ListenerFilterPlugin interface, which was used to generate
	// a list of NetworkFilters. That plugin wasn't implemented in the codebase so it
	// was removed. If we want to support other network filters, we would process
	// those plugins here.

	// Check that we don't refer to nonexistent auth config
	// TODO (sam-heilbron)
	// This is a partial duplicate of the open source ExtauthTranslatorSyncer
	// We should find a single place to define this configuration
	for i, vHost := range n.listener.GetVirtualHosts() {
		acRef := vHost.GetOptions().GetExtauth().GetConfigRef()
		if acRef != nil {
			if _, err := params.Snapshot.AuthConfigs.Find(acRef.GetNamespace(), acRef.GetName()); err != nil {
				validation.AppendVirtualHostError(
					n.report.GetVirtualHostReports()[i],
					validationapi.VirtualHostReport_Error_ProcessingError,
					"auth config not found: "+acRef.String())
			}
		}
	}

	// Process && sort the network filters.
	networkFilters := sortNetworkFilters(n.computePreHCMFilters(params))

	// add the http connection manager filter after all the InAuth Listener Filters
	networkFilter, err := n.hcmNetworkFilterTranslator.ComputeNetworkFilter(params)
	if err != nil {
		return nil, err
	}
	networkFilters = append(networkFilters, networkFilter)
	return networkFilters, nil
}

func sortNetworkFilters(filters plugins.StagedNetworkFilterList) []*envoy_config_listener_v3.Filter {
	sort.Sort(filters)
	var sortedFilters []*envoy_config_listener_v3.Filter
	for _, filter := range filters {
		sortedFilters = append(sortedFilters, filter.NetworkFilter)
	}
	return sortedFilters
}

type hcmNetworkFilterTranslator struct {
	parentListener *v1.Listener
	// A Gloo HttpListener which contains HttpConnectionManager settings
	listener *v1.HttpListener
	// The report where warnings/errors are persisted
	report *validationapi.HttpListenerReport
	// List of HttpFilterPlugins to process
	httpPlugins []plugins.HttpFilterPlugin
	// List of HttpConnectionManagerPlugins to process
	hcmPlugins []plugins.HttpConnectionManagerPlugin
	// The name of the RouteConfiguration for the HttpConnectionManager
	routeConfigName string
}

func (h *hcmNetworkFilterTranslator) ComputeNetworkFilter(params plugins.Params) (*envoy_config_listener_v3.Filter, error) {
	params.Ctx = contextutils.WithLogger(params.Ctx, "compute_http_connection_manager")

	// 1. Initialize the HCM
	httpConnectionManager := h.initializeHCM()

	// 2. Apply HttpFilters
	var err error
	httpConnectionManager.HttpFilters = h.computeHttpFilters(params)

	// 3. Allow any HCM plugins to make their changes, with respect to any changes the core plugin made
	for _, hcmPlugin := range h.hcmPlugins {
		if err := hcmPlugin.ProcessHcmNetworkFilter(params, h.parentListener, h.listener, httpConnectionManager); err != nil {
			validation.AppendHTTPListenerError(h.report,
				validationapi.HttpListenerReport_Error_ProcessingError,
				err.Error())
		}
	}

	// 4. Generate the typedConfig for the HCM
	hcmFilter, err := NewFilterWithTypedConfig(wellknown.HTTPConnectionManager, httpConnectionManager)
	if err != nil {
		contextutils.LoggerFrom(params.Ctx).DPanic("failed to convert proto message to struct")
		return nil, errors.Wrapf(err, "failed to convert proto message to struct")
	}

	return hcmFilter, nil
}

func (h *hcmNetworkFilterTranslator) initializeHCM() *envoyhttp.HttpConnectionManager {
	statPrefix := h.listener.GetStatPrefix()
	if statPrefix == "" {
		statPrefix = DefaultHttpStatPrefix
	}

	return &envoyhttp.HttpConnectionManager{
		CodecType:  envoyhttp.HttpConnectionManager_AUTO,
		StatPrefix: statPrefix,
		NormalizePath: &wrappers.BoolValue{
			Value: true,
		},
		RouteSpecifier: &envoyhttp.HttpConnectionManager_Rds{
			Rds: &envoyhttp.Rds{
				ConfigSource: &envoy_config_core_v3.ConfigSource{
					ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
					ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_Ads{
						Ads: &envoy_config_core_v3.AggregatedConfigSource{},
					},
				},
				RouteConfigName: h.routeConfigName,
			},
		},
	}
}

func (h *hcmNetworkFilterTranslator) computeHttpFilters(params plugins.Params) []*envoyhttp.HttpFilter {
	var httpFilters []plugins.StagedHttpFilter

	// run the HttpFilter Plugins
	for _, plug := range h.httpPlugins {
		stagedFilters, err := plug.HttpFilters(params, h.listener)
		if err != nil {
			validation.AppendHTTPListenerError(h.report, validationapi.HttpListenerReport_Error_ProcessingError, err.Error())
		}

		for _, httpFilter := range stagedFilters {
			if httpFilter.HttpFilter == nil {
				log.Warnf("plugin %v implements HttpFilters() but returned nil", plug.Name())
				continue
			}
			httpFilters = append(httpFilters, httpFilter)
		}
	}

	// https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/http/http_filters#filter-ordering
	// HttpFilter ordering determines the order in which the HCM will execute the filter.

	// 1. Sort filters by stage
	// "Stage" is the type we use to specify when a filter should be run
	envoyHttpFilters := sortHttpFilters(httpFilters)

	// 2. Configure the router filter
	// As outlined by the Envoy docs, the last configured filter has to be a terminal filter.
	// We set the Router filter (https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/router_filter#config-http-filters-router)
	// as the terminal filter in Gloo Edge.
	routerV3 := routerv3.Router{}

	if h.listener.GetOptions().GetRouter().GetSuppressEnvoyHeaders().GetValue() {
		routerV3.SuppressEnvoyHeaders = true
	}

	newStagedFilter, err := plugins.NewStagedFilter(
		wellknown.Router,
		&routerV3,
		plugins.AfterStage(plugins.RouteStage),
	)

	if err != nil {
		validation.AppendHTTPListenerError(h.report, validationapi.HttpListenerReport_Error_ProcessingError, err.Error())
	}

	envoyHttpFilters = append(envoyHttpFilters, newStagedFilter.HttpFilter)

	return envoyHttpFilters
}

func sortHttpFilters(filters plugins.StagedHttpFilterList) []*envoyhttp.HttpFilter {
	sort.Sort(filters)
	var sortedFilters []*envoyhttp.HttpFilter
	for _, filter := range filters {
		sortedFilters = append(sortedFilters, filter.HttpFilter)
	}
	return sortedFilters
}
