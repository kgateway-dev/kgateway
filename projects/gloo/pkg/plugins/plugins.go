package plugins

import (
	"context"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
)

// Plugin is a named unit of translation, used to produce Envoy configuration
type Plugin interface {
	// Name returns a unique identifier for a plugin
	Name() string

	// Init is used to re-initialize plugins and is executed for each translation loop
	// This is done for 2 reasons:
	//	1. Each translation run relies on its own context. If a plugin spawns a go-routine
	//		we need to be able to cancel that go-routine on the next translation
	//	2. Plugins are built with the assumption that they will be short lived, only for the
	//		duration of a single translation loop
	Init(params InitParams)
}

/*
	Params
*/

type InitParams struct {
	Ctx      context.Context
	Settings *v1.Settings
}

type Params struct {
	Ctx      context.Context
	Snapshot *v1snap.ApiSnapshot
	Messages map[*core.ResourceRef][]string
}

// CopyWithoutContext returns a version of params without ctx
// Mainly should be used for tests.
// Still copies pointer to snapshot.
func (p Params) CopyWithoutContext() Params {
	out := Params{
		Ctx:      context.Background(),
		Snapshot: p.Snapshot,
		Messages: map[*core.ResourceRef][]string{},
	}

	for k, v := range p.Messages {
		out.Messages[k] = v
	}

	return out
}

type VirtualHostParams struct {
	Params
	Proxy        *v1.Proxy
	Listener     *v1.Listener
	HttpListener *v1.HttpListener
}

type RouteParams struct {
	VirtualHostParams
	VirtualHost *v1.VirtualHost
}

type RouteActionParams struct {
	RouteParams
	Route *v1.Route
}

/*
	Upstream Plugins
*/

// UpstreamPlugin modifies the Envoy Cluster which has been created for the input Gloo Upstream.
// This allows the Cluster to be edited before being sent to Envoy via CDS
type UpstreamPlugin interface {
	Plugin
	ProcessUpstream(params Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error
}

// EndpointPlugin modifies an Envoy ClusterLoadAssignment (formerly known as an Endpoint) which
// has been created for the input Gloo Upstream.
// This allows the ClusterLoadAssignments to be edited before being sent to Envoy via EDS.
// NOTE: If one wishes to also modify the corresponding envoy Cluster the above UpstreamPlugin interface should be used.
type EndpointPlugin interface {
	Plugin
	ProcessEndpoints(params Params, in *v1.Upstream, out *envoy_config_endpoint_v3.ClusterLoadAssignment) error
}

/*
	Routing Plugins
*/

// RoutePlugin modifies an Envoy Route which has been created for the input Gloo Route.
// This allows the routes in a RouteConfiguration to be edited before being send to Envoy via RDS.
type RoutePlugin interface {
	Plugin
	ProcessRoute(params RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error
}

// RouteActionPlugin modifies an Envoy RouteAction which has been created for the input Gloo Route.
// NOTE: any route action plugin can be implemented as a route plugin
// suggestion: if your plugin requires configuration from a RoutePlugin field, implement the RoutePlugin interface
type RouteActionPlugin interface {
	Plugin
	ProcessRouteAction(params RouteActionParams, inAction *v1.RouteAction, out *envoy_config_route_v3.RouteAction) error
}

type WeightedDestinationPlugin interface {
	Plugin
	ProcessWeightedDestination(
		params RouteParams,
		in *v1.WeightedDestination,
		out *envoy_config_route_v3.WeightedCluster_ClusterWeight,
	) error
}

/*
	Listener Plugins
*/

type ListenerPlugin interface {
	Plugin
	ProcessListener(params Params, in *v1.Listener, out *envoy_config_listener_v3.Listener) error
}

type FilterChainMutatorPlugin interface {
	ListenerPlugin // TODO change this to Plugin, and update the places it's used
	ProcessFilterChain(params Params, in *v1.Listener, inFilters []*ExtendedFilterChain, out *envoy_config_listener_v3.Listener) error
}

type TcpFilterChainPlugin interface {
	Plugin
	CreateTcpFilterChains(params Params, parentListener *v1.Listener, in *v1.TcpListener) ([]*envoy_config_listener_v3.FilterChain, error)
}

// HttpConnectionManager Plugins
type HttpConnectionManagerPlugin interface {
	Plugin
	ProcessHcmNetworkFilter(params Params, parentListener *v1.Listener, listener *v1.HttpListener, out *envoyhttp.HttpConnectionManager) error
}

type HttpFilterPlugin interface {
	Plugin
	HttpFilters(params Params, listener *v1.HttpListener) ([]StagedHttpFilter, error)
}

type NetworkFilterPlugin interface {
	Plugin
	NetworkFiltersHTTP(params Params, listener *v1.HttpListener) ([]StagedNetworkFilter, error)
	NetworkFiltersTCP(params Params, listener *v1.TcpListener) ([]StagedNetworkFilter, error)
}

type VirtualHostPlugin interface {
	Plugin
	ProcessVirtualHost(params VirtualHostParams, in *v1.VirtualHost, out *envoy_config_route_v3.VirtualHost) error
}

/*
	Generation plugins
*/

// ResourceGeneratorPlugin modifies a set of xDS resources before they are persisted as a Snapshot
type ResourceGeneratorPlugin interface {
	Plugin
	GeneratedResources(params Params,
		inClusters []*envoy_config_cluster_v3.Cluster,
		inEndpoints []*envoy_config_endpoint_v3.ClusterLoadAssignment,
		inRouteConfigurations []*envoy_config_route_v3.RouteConfiguration,
		inListeners []*envoy_config_listener_v3.Listener,
	) ([]*envoy_config_cluster_v3.Cluster, []*envoy_config_endpoint_v3.ClusterLoadAssignment, []*envoy_config_route_v3.RouteConfiguration, []*envoy_config_listener_v3.Listener, error)
}

// A PluginRegistry is used to provide Plugins to relevant translators
// Historically, all plugins were passed around as an argument, and each translator
// would iterate over all plugins, and only apply the relevant ones.
// This interface enables translators to only know of the relevant plugins
type PluginRegistry interface {
	GetPlugins() []Plugin
	GetListenerPlugins() []ListenerPlugin
	GetTcpFilterChainPlugins() []TcpFilterChainPlugin
	GetHttpFilterPlugins() []HttpFilterPlugin
	GetNetworkFilterPlugins() []NetworkFilterPlugin
	GetHttpConnectionManagerPlugins() []HttpConnectionManagerPlugin
	GetVirtualHostPlugins() []VirtualHostPlugin
	GetResourceGeneratorPlugins() []ResourceGeneratorPlugin
	GetUpstreamPlugins() []UpstreamPlugin
	GetEndpointPlugins() []EndpointPlugin
	GetRoutePlugins() []RoutePlugin
	GetRouteActionPlugins() []RouteActionPlugin
	GetWeightedDestinationPlugins() []WeightedDestinationPlugin
}

// A PluginRegistryFactory generates a PluginRegistry
// It is executed each translation loop, ensuring we have up to date configuration of all plugins
type PluginRegistryFactory func(ctx context.Context) PluginRegistry

// ExtendedFilterChain is a FilterChain with additional information
// This extra information may not end up on the final filter chain
// But may be used to compute other aspects of the listener that are
// pulled along with filter chain.
type ExtendedFilterChain struct {
	*envoy_config_listener_v3.FilterChain
	PassthroughCipherSuites []string
	TerminatingCipherSuites []string
}
