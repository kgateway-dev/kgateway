package cors

import (
	"strings"

	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

type plugin struct {
}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *plugin) ProcessVirtualHost(params plugins.Params, in *v1.VirtualHost, out *envoyroute.VirtualHost) error {
	out.Cors = &envoyroute.CorsPolicy{}
	return p.translateUserCorsConfig(in.CorsPolicy, out.Cors)
}

func (p *plugin) translateUserCorsConfig(in *v1.CorsPolicy, out *envoyroute.CorsPolicy) error {
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
