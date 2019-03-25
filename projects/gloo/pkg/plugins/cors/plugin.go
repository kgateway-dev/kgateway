package cors

import (
	"fmt"
	"strings"

	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

type plugin struct {
}

var _ plugins.Plugin = new(plugin)
var _ plugins.HttpFilterPlugin = new(plugin)

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *plugin) ProcessVirtualHost(params plugins.Params, in *v1.VirtualHost, out *envoyroute.VirtualHost) error {
	if in.CorsPolicy == nil {
		return nil
	}
	out.Cors = &envoyroute.CorsPolicy{}
	return p.translateUserCorsConfig(in.CorsPolicy, out.Cors)
}

func (p *plugin) translateUserCorsConfig(in *v1.CorsPolicy, out *envoyroute.CorsPolicy) error {
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
