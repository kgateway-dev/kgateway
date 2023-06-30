package cors

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_config_cors_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"

	regexutils "github.com/solo-io/gloo/pkg/utils/regexutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/cors"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
)

var (
	_ plugins.Plugin            = new(plugin)
	_ plugins.HttpFilterPlugin  = new(plugin)
	_ plugins.RoutePlugin       = new(plugin)
	_ plugins.VirtualHostPlugin = new(plugin)
)

const (
	ExtensionName = "cors"
)

var (
	InvalidRouteActionError = errors.New("cannot use cors plugin on non-Route_Route route actions")
	pluginStage             = plugins.DuringStage(plugins.CorsStage)
)

type plugin struct {
	removeUnused              bool
	filterRequiredForListener map[*v1.HttpListener]struct{}
}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) {
	p.removeUnused = params.Settings.GetGloo().GetRemoveUnusedFilters().GetValue()
	p.filterRequiredForListener = make(map[*v1.HttpListener]struct{})
}

func (p *plugin) ProcessVirtualHost(
	params plugins.VirtualHostParams,
	in *v1.VirtualHost,
	out *envoy_config_route_v3.VirtualHost,
) error {
	corsPlugin := in.GetOptions().GetCors()
	if corsPlugin == nil {
		return nil
	}
	if corsPlugin.GetDisableForRoute() {
		contextutils.LoggerFrom(params.Ctx).Warnw(
			"invalid virtual host cors policy: DisableForRoute only pertains to cors policies on routes",
			zap.Any("virtual host", in.GetName()),
		)
	}
	p.filterRequiredForListener[params.HttpListener] = struct{}{}
	corsPolicy := &envoy_config_cors_v3.CorsPolicy{}
	p.translateCommonUserCorsConfig(params.Ctx, corsPlugin, corsPolicy)

	return pluginutils.SetVhostPerFilterConfig(out, wellknown.CORS, corsPolicy)
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	corsPlugin := in.GetOptions().GetCors()
	if corsPlugin == nil {
		return nil
	}

	// if the route has a direct response action, the cors filter will not apply headers to the response
	// instead, configure ResponseHeadersToAdd on the direct response action
	if _, ok := out.GetAction().(*envoy_config_route_v3.Route_DirectResponse); ok &&
		!corsPlugin.GetDisableForRoute() {
		out.ResponseHeadersToAdd = append(out.GetResponseHeadersToAdd(), getCorsResponseHeadersFromPolicy(corsPlugin)...)
		return nil
	}

	// the cors filter can only be used on routes that are of type envoyroute.Route_Route
	if out.GetAction() != nil && out.GetRoute() == nil {
		return InvalidRouteActionError
	}
	// we have already ensured that the output route action is either nil or of the proper type
	// if it is nil, we initialize it prior to transforming it
	outRa := out.GetRoute()
	if outRa == nil {
		out.Action = &envoy_config_route_v3.Route_Route{
			Route: &envoy_config_route_v3.RouteAction{},
		}
		outRa = out.GetRoute()
	}

	p.filterRequiredForListener[params.HttpListener] = struct{}{}
	corsPolicy := &envoy_config_cors_v3.CorsPolicy{}
	if err := p.translateCommonUserCorsConfig(params.Ctx, in.GetOptions().GetCors(), corsPolicy); err != nil {
		return err
	}
	p.translateRouteSpecificCorsConfig(in.GetOptions().GetCors(), corsPolicy)

	return pluginutils.SetRoutePerFilterConfig(out, wellknown.CORS, corsPolicy)
}

func (p *plugin) translateCommonUserCorsConfig(
	ctx context.Context,
	in *cors.CorsPolicy,
	out *envoy_config_cors_v3.CorsPolicy,
) error {
	if len(in.GetAllowOrigin()) == 0 && len(in.GetAllowOriginRegex()) == 0 {
		return fmt.Errorf("must provide at least one of AllowOrigin or AllowOriginRegex")
	}
	for _, ao := range in.GetAllowOrigin() {
		out.AllowOriginStringMatch = append(out.GetAllowOriginStringMatch(), &envoy_type_matcher_v3.StringMatcher{
			MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{Exact: ao},
		})
	}
	for _, ao := range in.GetAllowOriginRegex() {
		out.AllowOriginStringMatch = append(out.GetAllowOriginStringMatch(), &envoy_type_matcher_v3.StringMatcher{
			MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{SafeRegex: regexutils.NewRegex(ctx, ao)},
		})
	}
	out.AllowMethods = strings.Join(in.GetAllowMethods(), ",")
	out.AllowHeaders = strings.Join(in.GetAllowHeaders(), ",")
	out.ExposeHeaders = strings.Join(in.GetExposeHeaders(), ",")
	out.MaxAge = in.GetMaxAge()
	if in.GetAllowCredentials() {
		out.AllowCredentials = &wrappers.BoolValue{Value: in.GetAllowCredentials()}
	}
	return nil
}

// not expecting this to be used
const runtimeKey = "gloo.routeplugin.cors"

func (p *plugin) translateRouteSpecificCorsConfig(in *cors.CorsPolicy, out *envoy_config_cors_v3.CorsPolicy) {
	if in.GetDisableForRoute() {
		out.FilterEnabled = &envoy_config_core_v3.RuntimeFractionalPercent{
			DefaultValue: &envoy_type_v3.FractionalPercent{
				Numerator:   0,
				Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
			},
			RuntimeKey: runtimeKey,
		}
	}
}

func (p *plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	_, ok := p.filterRequiredForListener[listener]
	if !ok && p.removeUnused {
		return []plugins.StagedHttpFilter{}, nil
	}

	return []plugins.StagedHttpFilter{plugins.MustNewStagedFilter(wellknown.CORS, &envoy_config_cors_v3.Cors{}, pluginStage)}, nil
}

// convert allowOrigin and allowOriginRegex options to a deduplicated slice of strings
func convertAllowOriginToSlice(corsPolicy *cors.CorsPolicy) []string {
	exists := struct{}{}
	allowOriginSet := make(map[string]struct{})
	for _, origin := range corsPolicy.GetAllowOrigin() {
		allowOriginSet[origin] = exists
	}
	for _, originRegex := range corsPolicy.GetAllowOriginRegex() {
		allowOriginSet[originRegex] = exists
	}

	// concatenate the allow origin set into a string
	allowedOrigins := []string{}
	for origin := range allowOriginSet {
		allowedOrigins = append(allowedOrigins, origin)
	}

	return allowedOrigins
}

// get response headers to add from cors policy
// this is only used when processing direct response actions, for which
// the cors filter is disabled
func getCorsResponseHeadersFromPolicy(corsPolicy *cors.CorsPolicy) []*envoy_config_core_v3.HeaderValueOption {
	allowOriginString := strings.Join(convertAllowOriginToSlice(corsPolicy), ",")

	return []*envoy_config_core_v3.HeaderValueOption{
		{
			Header: &envoy_config_core_v3.HeaderValue{
				Key:   "Access-Control-Allow-Origin",
				Value: allowOriginString,
			},
			KeepEmptyValue: false,
		},
		{
			Header: &envoy_config_core_v3.HeaderValue{
				Key:   "Access-Control-Allow-Methods",
				Value: strings.Join(corsPolicy.GetAllowMethods(), ","),
			},
			KeepEmptyValue: false,
		},
		{
			Header: &envoy_config_core_v3.HeaderValue{
				Key:   "Access-Control-Allow-Headers",
				Value: strings.Join(corsPolicy.GetAllowHeaders(), ","),
			},
			KeepEmptyValue: false,
		},
		{
			Header: &envoy_config_core_v3.HeaderValue{
				Key:   "Access-Control-Expose-Headers",
				Value: strings.Join(corsPolicy.GetExposeHeaders(), ","),
			},
			KeepEmptyValue: false,
		},
		{
			Header: &envoy_config_core_v3.HeaderValue{
				Key:   "Access-Control-Max-Age",
				Value: corsPolicy.GetMaxAge(),
			},
			KeepEmptyValue: false,
		},
		{
			Header: &envoy_config_core_v3.HeaderValue{
				Key:   "Access-Control-Allow-Credentials",
				Value: strconv.FormatBool(corsPolicy.GetAllowCredentials()),
			},
			KeepEmptyValue: false,
		},
	}
}
