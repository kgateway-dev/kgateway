package virtualhost

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var (
	_ plugins.Plugin            = new(plugin)
	_ plugins.VirtualHostPlugin = new(plugin)
)

const (
	ExtensionName = "virtual_host"
)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(_ plugins.InitParams) {
}

func (p *plugin) ProcessVirtualHost(
	params plugins.VirtualHostParams,
	in *v1.VirtualHost,
	out *envoy_config_route_v3.VirtualHost,
) error {
	// Both these values default to false if unset, so there's need to set anything if input is nil.
	// (Input is a google.protobuf.BoolValue, so it can be true, false, or nil)
	if irac := in.GetOptions().GetIncludeRequestAttemptCount(); irac != nil {
		out.IncludeRequestAttemptCount = irac.GetValue()
	}
	if iacir := in.GetOptions().GetIncludeAttemptCountInResponse(); iacir != nil {
		out.IncludeAttemptCountInResponse = iacir.GetValue()
	}
	return nil
}
