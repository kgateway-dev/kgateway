package extauth

import (
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

const (
	FilterName        = "envoy.ext_authz"
	DefaultAuthHeader = "x-user-id"
	HttpServerUri     = "http://not-used.example.com/"
)

// Note that although this configures the "envoy.ext_authz" filter, we still want the ordering to be within the
// AuthNStage because we are using this filter for authentication purposes
var filterStage = plugins.DuringStage(plugins.AuthNStage)

var _ plugins.Plugin = &Plugin{}

func NewCustomAuthPlugin() *Plugin {
	return &Plugin{}
}

type Plugin struct {
	extAuthSettings *extauthv1.Settings
}

func (p *Plugin) Init(params plugins.InitParams) error {
	p.extAuthSettings = params.Settings.GetExtauth()
	return nil
}

func (p *Plugin) HttpFilters(params plugins.Params, _ *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	// Delegate to a function with a simpler signature,will make it easier to reuse
	return BuildHttpFilters(p.extAuthSettings, params.Snapshot.Upstreams)
}

func (p *Plugin) ProcessVirtualHost(params plugins.VirtualHostParams, in *v1.VirtualHost, out *route.VirtualHost) error {
	return nil
}

func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *route.Route) error {
	return nil
}

func (p *Plugin) ProcessWeightedDestination(params plugins.RouteParams, in *v1.WeightedDestination, out *route.WeightedCluster_ClusterWeight) error {
	return nil
}
