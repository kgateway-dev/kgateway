package grpcweb

import (
	envoygrpcweb "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/grpc_web/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var (
	_ plugins.Plugin           = new(plugin)
	_ plugins.HttpFilterPlugin = new(plugin)
)

const (
	ExtensionName = "grpc_web"
)

// filter info
var pluginStage = plugins.AfterStage(plugins.AuthZStage)

type plugin struct {
	disabled bool
}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) {
	maybeDisabled := params.Settings.GetGloo().GetDisableGrpcWeb()
	if maybeDisabled != nil {
		p.disabled = maybeDisabled.GetValue()
	} else {
		// default to the state of RemoveUnusedFilters, if unspecified
		// this is a safe fallback because this value defaults to false
		p.disabled = params.Settings.GetGloo().GetRemoveUnusedFilters().GetValue()
	}
}

func (p *plugin) isDisabled(httplistener *v1.HttpListener) bool {
	grpcWeb := httplistener.GetOptions().GetGrpcWeb()

	if grpcWeb == nil {
		// There is no configured defined on this listener, fallback to the settings
		return p.disabled
	}
	return grpcWeb.GetDisable()
}

// HttpFilters will add an empty version of the grpcweb filter to a listener
// if it is not explicitly set.
func (p *plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	if p.isDisabled(listener) {
		return nil, nil
	}

	return []plugins.StagedHttpFilter{plugins.MustNewStagedFilter(wellknown.GRPCWeb, &envoygrpcweb.GrpcWeb{}, pluginStage)}, nil
}
