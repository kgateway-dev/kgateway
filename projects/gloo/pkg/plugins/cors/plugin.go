package cors

import (
	"errors"
	"fmt"
	"strings"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/util"
	"go.uber.org/zap"

	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"

	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoymatcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/cors"
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
	corsPlugin := in.Options.GetCors()
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
	corsPlugin := in.Options.GetCors()
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
	if err := p.translateCommonUserCorsConfig(in.Options.Cors, outRa.Cors); err != nil {
		return err
	}
	p.translateRouteSpecificCorsConfig(in.Options.Cors, outRa.Cors)
	return nil
}

func (p *plugin) translateCommonUserCorsConfig(in *cors.CorsPolicy, out *envoyroute.CorsPolicy) error {
	if len(in.AllowOrigin) == 0 && len(in.AllowOriginRegex) == 0 {
		return fmt.Errorf("must provide at least one of AllowOrigin or AllowOriginRegex")
	}
	for _, ao := range in.AllowOrigin {
		out.AllowOriginStringMatch = append(out.AllowOriginStringMatch, &envoymatcher.StringMatcher{
			MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: ao},
		})
	}
	for _, ao := range in.AllowOriginRegex {
		out.AllowOriginStringMatch = append(out.AllowOriginStringMatch, &envoymatcher.StringMatcher{
			MatchPattern: &envoymatcher.StringMatcher_SafeRegex{SafeRegex: &envoymatcher.RegexMatcher{
				EngineType: &envoymatcher.RegexMatcher_GoogleRe2{},
				Regex:      ao,
			}},
		})
	}
	out.AllowMethods = strings.Join(in.AllowMethods, ",")
	out.AllowHeaders = strings.Join(in.AllowHeaders, ",")
	out.ExposeHeaders = strings.Join(in.ExposeHeaders, ",")
	out.MaxAge = in.MaxAge
	if in.AllowCredentials {
		out.AllowCredentials = &wrappers.BoolValue{Value: in.AllowCredentials}
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
		plugins.NewStagedFilter(util.CORS, pluginStage),
	}, nil
}
