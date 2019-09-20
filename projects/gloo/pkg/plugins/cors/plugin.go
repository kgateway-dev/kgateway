package cors

import (
	"errors"
	"fmt"
	"strings"

	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"

	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/cors"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

type plugin struct {
}

var _ plugins.Plugin = new(plugin)
var _ plugins.HttpFilterPlugin = new(plugin)
var _ plugins.RoutePlugin = new(plugin)

var (
	InvalidRouteActionError = errors.New("cannot use cors plugin on non-Route_Route route actions")
)

var pluginStage = plugins.DuringStage(plugins.CorsStage)

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *plugin) ProcessVirtualHost(params plugins.VirtualHostParams, in *v1.VirtualHost, out *envoyroute.VirtualHost) error {
	corsPlugin := in.VirtualHostPlugins.GetCors()

	// remove this block when deprecated v1.CorsPolicy API is removed
	if in.CorsPolicy != nil {
		if corsPlugin == nil {
			out.Cors = &envoyroute.CorsPolicy{}
			return p.translateCommonUserCorsConfig(convertDeprecatedCorsPolicy(in.CorsPolicy), out.Cors)
		} else {
			contextutils.LoggerFrom(params.Ctx).Warnw("multiple CorsPolicies specified. Ignoring deprecated"+
				" CorsPolicy field and using VirtualHostPlugins.Cors spec",
				zap.Any("virtual host", in.Name))
			// fallthrough and use the virtual host plugin spec instead of the deprecated field
		}
	}

	if corsPlugin == nil {
		return nil
	}
	if corsPlugin.DisableForRoute {
		contextutils.LoggerFrom(params.Ctx).Warnw("invalid virtual host cors policy: DisableForRoute only pertains to cors policies on routes",
			zap.Any("virtual host", in.Name))
	}
	out.Cors = &envoyroute.CorsPolicy{}
	return p.translateCommonUserCorsConfig(corsPlugin, out.Cors)
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoyroute.Route) error {
	corsPlugin := in.RoutePlugins.GetCors()
	if corsPlugin == nil {
		return nil
	}
	// the cors plugin should only be used on routes that are of type envoyroute.Route_Route
	if out.Action != nil && out.GetRoute() == nil {
		return InvalidRouteActionError
	}
	// we have already ensured that the output route action is either nil or of the proper type
	// if it is nil, we initialize it prior to transforming it
	outRa := out.GetRoute()
	if outRa == nil {
		out.Action = &envoyroute.Route_Route{
			Route: &envoyroute.RouteAction{},
		}
		outRa = out.GetRoute()
	}
	outRa.Cors = &envoyroute.CorsPolicy{}
	if err := p.translateCommonUserCorsConfig(in.RoutePlugins.Cors, outRa.Cors); err != nil {
		return err
	}
	p.translateRouteSpecificCorsConfig(in.RoutePlugins.Cors, outRa.Cors)
	return nil
}

func (p *plugin) translateCommonUserCorsConfig(in *cors.CorsPolicy, out *envoyroute.CorsPolicy) error {
	if len(in.AllowOrigin) == 0 && len(in.AllowOriginRegex) == 0 {
		return fmt.Errorf("must provide at least one of AllowOrigin or AllowOriginRegex")
	}
	out.AllowOrigin = in.AllowOrigin
	out.AllowOriginRegex = in.AllowOriginRegex
	out.AllowMethods = strings.Join(in.AllowMethods, ",")
	out.AllowHeaders = strings.Join(in.AllowHeaders, ",")
	out.ExposeHeaders = strings.Join(in.ExposeHeaders, ",")
	out.MaxAge = in.MaxAge
	if in.AllowCredentials {
		out.AllowCredentials = &types.BoolValue{Value: in.AllowCredentials}
	}
	return nil
}

// not expecting this to be used
const runtimeKey = "gloo.routeplugin.cors"

func (p *plugin) translateRouteSpecificCorsConfig(in *cors.CorsPolicy, out *envoyroute.CorsPolicy) {
	if in.DisableForRoute {
		out.EnabledSpecifier = &envoyroute.CorsPolicy_FilterEnabled{
			FilterEnabled: &core.RuntimeFractionalPercent{
				DefaultValue: &envoy_type.FractionalPercent{
					Numerator:   0,
					Denominator: envoy_type.FractionalPercent_HUNDRED,
				},
				RuntimeKey: runtimeKey,
			},
		}
	}
}

func (p *plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	return []plugins.StagedHttpFilter{
		plugins.NewStagedFilter(envoyutil.CORS, pluginStage),
	}, nil
}

func convertDeprecatedCorsPolicy(in *v1.CorsPolicy) *cors.CorsPolicy {
	out := &cors.CorsPolicy{}
	if in == nil {
		return out
	}
	out.AllowCredentials = in.AllowCredentials
	out.AllowHeaders = in.AllowHeaders
	out.AllowOrigin = in.AllowOrigin
	out.AllowOriginRegex = in.AllowOriginRegex
	out.AllowMethods = in.AllowMethods
	out.AllowHeaders = in.AllowHeaders
	out.ExposeHeaders = in.ExposeHeaders
	out.MaxAge = in.MaxAge
	out.AllowCredentials = in.AllowCredentials
	return out
}
