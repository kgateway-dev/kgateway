package extauth

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
)

var (
	_ plugins.Plugin                    = new(plugin)
	_ plugins.HttpFilterPlugin          = new(plugin)
	_ plugins.VirtualHostPlugin         = new(plugin)
	_ plugins.RoutePlugin               = new(plugin)
	_ plugins.WeightedDestinationPlugin = new(plugin)
)

const (
	DefaultAuthHeader = "x-user-id"
	HttpServerUri     = "http://not-used.example.com/"
	ExtensionName     = "ext_authz"
)

// Note that although this configures the "envoy.filters.http.ext_authz" filter, we still want the ordering to be within the
// AuthNStage because we are using this filter for authentication purposes
var FilterStage = plugins.DuringStage(plugins.AuthNStage)

func NewPlugin() *plugin {
	return &plugin{}
}

type plugin struct {
	extAuthzConfigGenerator ExtAuthzConfigGenerator
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) {
	p.extAuthzConfigGenerator = getOpenSourceConfigGenerator(params.Settings.GetExtauth(), params.Settings.GetNamedExtauth())
}

func (p *plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	return BuildStagedHttpFilters(func() ([]*envoyauth.ExtAuthz, error) {
		return p.extAuthzConfigGenerator.GenerateListenerExtAuthzConfig(listener, params.Snapshot.Upstreams)
	}, FilterStage)
}

// This function generates the ext_authz TypedPerFilterConfig for this virtual host. If the ext_authz filter was not
// configured on the listener, do nothing. If the filter is configured and the virtual host does not define
// an extauth configuration OR explicitly disables extauth, we disable the ext_authz filter.
// This is done to disable authentication by default on a virtual host and its child resources (routes, weighted
// destinations). Extauth is currently opt-in.
func (p *plugin) ProcessVirtualHost(
	params plugins.VirtualHostParams,
	in *v1.VirtualHost,
	out *envoy_config_route_v3.VirtualHost,
) error {

	// Ext_authz filter is not configured on listener, do nothing
	if !p.isExtAuthzFilterConfigured(params.HttpListener, params.Snapshot.Upstreams) {
		return nil
	}

	extAuthPerRouteConfig, err := p.extAuthzConfigGenerator.GenerateVirtualHostExtAuthzConfig(in, params)
	if err != nil {
		return err
	}
	if extAuthPerRouteConfig == nil {
		return nil
	}

	return pluginutils.SetVhostPerFilterConfig(out, wellknown.HTTPExternalAuthorization, extAuthPerRouteConfig)
}

// This function generates the ext_authz TypedPerFilterConfig for this route:
// - if the route defines custom auth configuration, set the filter correspondingly;
// - if auth is explicitly disabled, disable the filter (will apply by default also to WeightedDestinations);
// - else, do nothing (will inherit config from parent virtual host).
func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {

	// Ext_authz filter is not configured on listener, do nothing
	if !p.isExtAuthzFilterConfigured(params.HttpListener, params.Snapshot.Upstreams) {
		return nil
	}

	extAuthPerRouteConfig, err := p.extAuthzConfigGenerator.GenerateRouteExtAuthzConfig(in)
	if err != nil {
		return err
	}
	if extAuthPerRouteConfig == nil {
		return nil
	}

	return pluginutils.SetRoutePerFilterConfig(out, wellknown.HTTPExternalAuthorization, extAuthPerRouteConfig)
}

// This function generates the ext_authz TypedPerFilterConfig for this weightedDestination:
// - if the weightedDestination defines custom auth configuration, set the filter correspondingly;
// - if auth is explicitly disabled, disable the filter;
// - else, do nothing (will inherit config from parent virtual host and/or route).
func (p *plugin) ProcessWeightedDestination(
	params plugins.RouteParams,
	in *v1.WeightedDestination,
	out *envoy_config_route_v3.WeightedCluster_ClusterWeight,
) error {

	// Ext_authz filter is not configured on listener, do nothing
	if !p.isExtAuthzFilterConfigured(params.HttpListener, params.Snapshot.Upstreams) {
		return nil
	}

	extAuthPerRouteConfig, err := p.extAuthzConfigGenerator.GenerateWeightedDestinationExtAuthzConfig(in)
	if err != nil {
		return err
	}
	if extAuthPerRouteConfig == nil {
		return nil
	}

	return pluginutils.SetWeightedClusterPerFilterConfig(out, wellknown.HTTPExternalAuthorization, extAuthPerRouteConfig)
}

func (p *plugin) isExtAuthzFilterConfigured(listener *v1.HttpListener, upstreams v1.UpstreamList) bool {
	// Call the same function called by HttpFilters to verify whether the filter was created
	stagedFilters, err := BuildStagedHttpFilters(func() ([]*envoyauth.ExtAuthz, error) {
		return p.extAuthzConfigGenerator.GenerateListenerExtAuthzConfig(listener, upstreams)
	}, FilterStage)

	if err != nil {
		// If it returned an error, the filter was not configured
		return false
	}

	// Check for a filter called "envoy.filters.http.ext_authz"
	return plugins.StagedFilterListContainsName(stagedFilters, wellknown.HTTPExternalAuthorization)
}
