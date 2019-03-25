package cors

import (
	"fmt"
	"strings"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type"
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
	out.EnabledSpecifier = &envoyroute.CorsPolicy_FilterEnabled{
		FilterEnabled: runtimeConstEnabled(),
	}
	return nil
}

// runtimeConstEnabled is a helper for setting a runtime fractional percent field to a constant enabled value.
// Useful for cases where you do not want a value to change during runtime even though envoy supports it.
func runtimeConstEnabled() *envoycore.RuntimeFractionalPercent {
	return &envoycore.RuntimeFractionalPercent{
		DefaultValue: &envoytype.FractionalPercent{
			// setting 100/...HUNDRED "provides" the enabled property
			Numerator:   100,
			Denominator: envoytype.FractionalPercent_HUNDRED,
		},
		// Disabling the RuntimeKey field provides the "const" property
		// RuntimeKey: "intentionally_disabled"
	}
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
