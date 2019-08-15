package cors

import (
	"errors"
	"fmt"
	"strings"

	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
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
	InvalidDualSpecError    = errors.New("invalid cors spec: must specify one of VirtualHostPlugins.Cors or CorsPolicy (deprecated) - both were provided")
	InvalidRouteActionError = errors.New("cannot use shadowing plugin on non-Route_Route route actions")
)

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *plugin) ProcessVirtualHost(params plugins.VirtualHostParams, in *v1.VirtualHost, out *envoyroute.VirtualHost) error {
	corsPlugin := in.VirtualHostPlugins.GetCors()
	if corsPlugin == nil && in.CorsPolicy == nil {
		return nil
	}
	if corsPlugin != nil && in.CorsPolicy != nil {
		return InvalidDualSpecError
	}
	out.Cors = &envoyroute.CorsPolicy{}
	if in.CorsPolicy != nil {
		return p.translateUserCorsConfig(convertDeprectedCorsPolicy(in.CorsPolicy), out.Cors)
	}
	return p.translateUserCorsConfig(corsPlugin, out.Cors)
}

func convertDeprectedCorsPolicy(in *v1.CorsPolicy) *cors.CorsPolicy {
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
	return p.translateUserCorsConfig(in.RoutePlugins.Cors, outRa.Cors)
}

func (p *plugin) translateUserCorsConfig(in *cors.CorsPolicy, out *envoyroute.CorsPolicy) error {
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

const (
	// filter info
	pluginStage = plugins.PostInAuth
)

func (p *plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	return []plugins.StagedHttpFilter{
		{
			HttpFilter: &envoyhttp.HttpFilter{Name: envoyutil.CORS},
			Stage:      pluginStage,
		},
	}, nil
}
