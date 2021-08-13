package shadowing

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/shadowing"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/internal/common"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
)

var (
	InvalidRouteActionError  = eris.New("cannot use shadowing plugin on non-Route_Route route actions")
	UnspecifiedUpstreamError = eris.New("invalid plugin spec: must specify an upstream ref")
	InvalidNumeratorError    = func(num float32) error {
		return eris.Errorf("shadow percentage must be between 0 and 100, received %v", num)
	}
)

func NewPlugin() *Plugin {
	return &Plugin{}
}

var _ plugins.Plugin = new(Plugin)
var _ plugins.RoutePlugin = new(Plugin)

type Plugin struct {
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	if in.GetOptions() == nil || in.GetOptions().GetShadowing() == nil {
		return nil
	}
	// the shadow plugin should only be used on routes that are of type envoyroute.Route_Route
	// (this is because shadowing is not defined on redirect or direct response route actions)
	if out.GetAction() != nil && out.GetRoute() == nil {
		return InvalidRouteActionError
	}
	shadowSpec := in.GetOptions().Shadowing
	// we have already ensured that the output route action is either nil or of the proper type
	// if it is nil, we initialize it prior to transforming it
	outRa := out.GetRoute()
	if outRa == nil {
		out.Action = &envoy_config_route_v3.Route_Route{
			Route: &envoy_config_route_v3.RouteAction{},
		}
		outRa = out.GetRoute()
	}
	return applyShadowSpec(outRa, shadowSpec)
}

func applyShadowSpec(out *envoy_config_route_v3.RouteAction, spec *shadowing.RouteShadowing) error {
	if spec.GetUpstream() == nil {
		return UnspecifiedUpstreamError
	}
	if spec.GetPercentage() < 0 || spec.GetPercentage() > 100 {
		return InvalidNumeratorError(spec.GetPercentage())
	}
	out.RequestMirrorPolicies = []*envoy_config_route_v3.RouteAction_RequestMirrorPolicy{
		{
			Cluster:         translator.UpstreamToClusterName(spec.GetUpstream()),
			RuntimeFraction: getFractionalPercent(spec.GetPercentage()),
		},
	}
	return nil
}

func getFractionalPercent(numerator float32) *envoy_config_core_v3.RuntimeFractionalPercent {
	return &envoy_config_core_v3.RuntimeFractionalPercent{
		DefaultValue: common.ToEnvoyPercentage(numerator),
	}
}
