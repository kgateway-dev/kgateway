package shadowing

import (
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/shadowing"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/go-utils/errors"
)

var (
	InvalidRouteActionError  = errors.New("cannot use shadowing plugin on non-Route_Route route actions")
	UnspecifiedUpstreamError = errors.New("invalid plugin spec: must specify an upstream ref")
	InvalidNumeratorError    = func(num uint32) error {
		return errors.Errorf("shadow percentage must be between 0 and 100, received %v", num)
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

func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoyroute.Route) error {
	if in.RoutePlugins == nil || in.RoutePlugins.Shadowing == nil {
		return nil
	}
	// the shadow plugin should only be used on routes that are of type envoyroute.Route_Route
	// (this is because shadowing is not defined on redirect or direct response route actions)
	if out.Action != nil && out.GetRoute() == nil {
		return InvalidRouteActionError
	}
	shadowSpec := in.RoutePlugins.Shadowing
	// we have already ensured that the output route action is either nil or of the proper type
	// if it is nil, we initialize it prior to transforming it
	outRa := out.GetRoute()
	if outRa == nil {
		out.Action = &envoyroute.Route_Route{
			Route: &envoyroute.RouteAction{},
		}
		outRa = out.GetRoute()
	}
	return applyShadowSpec(outRa, shadowSpec)
}

func applyShadowSpec(out *envoyroute.RouteAction, spec *shadowing.RouteShadowing) error {
	if spec.Upstream == nil {
		return UnspecifiedUpstreamError
	}
	if spec.Percent > 100 {
		return InvalidNumeratorError(spec.Percent)
	}
	out.RequestMirrorPolicy = &envoyroute.RouteAction_RequestMirrorPolicy{
		Cluster:         translator.UpstreamToClusterName(*spec.Upstream),
		RuntimeFraction: getFractionalPercent(spec.Percent),
	}
	return nil
}

func getFractionalPercent(numerator uint32) *envoycore.RuntimeFractionalPercent {
	return &envoycore.RuntimeFractionalPercent{
		DefaultValue: &envoy_type.FractionalPercent{
			Numerator:   numerator,
			Denominator: envoy_type.FractionalPercent_HUNDRED,
		},
	}
}
