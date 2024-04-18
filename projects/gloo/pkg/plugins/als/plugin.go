package als

import (
	"context"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/go-utils/contextutils"
)

var (
	_ plugins.Plugin                      = new(plugin)
	_ plugins.HttpConnectionManagerPlugin = new(plugin)
	_ plugins.ListenerPlugin              = new(plugin)
)

const (
	ExtensionName = "als"
	ClusterName   = "access_log_cluster"
)

// Access logging plugin can contain a context for logging
type plugin struct {
	ctx context.Context
}

// NewPlugin creates an empty als plugin with no extra data
func NewPlugin() *plugin {
	return &plugin{}
}

// Name returns "als"
func (p *plugin) Name() string {
	return ExtensionName
}

// Init grabs the context for logging
func (p *plugin) Init(params plugins.InitParams) {
	p.ctx = params.Ctx
}

// ProcessHcmNetworkFilter will configure access logging for the hcm.
// This delegates most of its logic to ProcessAccessLogPlugins, which is also used by the TCP plugin and the listener level configuration.
func (p *plugin) ProcessHcmNetworkFilter(params plugins.Params, parentListener *v1.Listener, _ *v1.HttpListener, out *envoyhttp.HttpConnectionManager) error {
	if out == nil {
		return nil
	}
	// AccessLog settings are defined on the root listener, and applied to each HCM instance
	alsSettings := parentListener.GetOptions().GetAccessLoggingService()
	if alsSettings == nil {
		return nil
	}

	var err error
	out.AccessLog, err = ProcessAccessLogPlugins(alsSettings, out.GetAccessLog())

	// TODO: Add extra warning calls. Access logging directives are "valid" for all possible configuration locations
	if err := DetectUnusefulCmds(Hcm, out.AccessLog); err != nil {
		contextutils.LoggerFrom(p.ctx).Warnf("warning non-useful access log operator: %v", err)
	}
	return err
}

// ProcessListener will configure access logging at the listener level.
func (p *plugin) ProcessListener(params plugins.Params, parentListener *v1.Listener, out *envoy_config_listener_v3.Listener) error {

	alsSettings := parentListener.GetOptions().GetEarlyAccessLoggingService()
	if alsSettings == nil {
		return nil
	}
	var err error
	out.AccessLog, err = ProcessAccessLogPlugins(alsSettings, out.GetAccessLog())

	// TODO: Add extra warning calls. Access logging directives are "valid" for all possible configuration locations
	if err := DetectUnusefulCmds(HttpListener, out.AccessLog); err != nil {
		contextutils.LoggerFrom(p.ctx).Warnf("warning non-useful access log operator: %v", err)
	}

	return err

}
