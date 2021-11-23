package extauth

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
)

const (
	DefaultAuthHeader = "x-user-id"
	HttpServerUri     = "http://not-used.example.com/"
	ExtensionName     = "ext_authz"
)

// Note that although this configures the "envoy.filters.http.ext_authz" filter, we still want the ordering to be within the
// AuthNStage because we are using this filter for authentication purposes
var FilterStage = plugins.DuringStage(plugins.AuthNStage)

var (
	_ plugins.Plugin                    = &Plugin{}
	_ plugins.HttpFilterPlugin          = &Plugin{}
	_ plugins.VirtualHostPlugin         = &Plugin{}
	_ plugins.RoutePlugin               = &Plugin{}
	_ plugins.WeightedDestinationPlugin = &Plugin{}
	_ plugins.Upgradable                = &Plugin{}
)

func NewCustomAuthPlugin() *Plugin {
	return &Plugin{}
}

type Plugin struct {
	extAuthzConfigGenerator ExtAuthzConfigGenerator
}

func (p *Plugin) Init(params plugins.InitParams) error {
	p.extAuthzConfigGenerator = getOpenSourceConfigGenerator(params.Settings.GetExtauth(), params.Settings.GetNamedExtauth())
	return nil
}

func (p *Plugin) PluginName() string {
	return ExtensionName
}

func (p *Plugin) IsUpgrade() bool {
	// Configuration for ext_authz filters have diverged enough between open and closed source gloo
	// that it makes sense to configure them separately
	return false
}

func (p *Plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	return BuildStagedHttpFilters(func() ([]*envoyauth.ExtAuthz, error) {
		return p.extAuthzConfigGenerator.GenerateListenerExtAuthzConfig(listener, params.Snapshot.Upstreams)
	}, FilterStage)
}

// This function generates the ext_authz TypedPerFilterConfig for this virtual host. If the ext_authz filter was not
// configured on the listener, do nothing. If the filter is configured and the virtual host does not define
// an extauth configuration OR explicitly disables extauth, we disable the ext_authz filter.
// This is done to disable authentication by default on a virtual host and its child resources (routes, weighted
// destinations). Extauth is currently opt-in.
func (p *Plugin) ProcessVirtualHost(
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
func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {

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
func (p *Plugin) ProcessWeightedDestination(
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

func (p *Plugin) isExtAuthzFilterConfigured(listener *v1.HttpListener, upstreams v1.UpstreamList) bool {
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
