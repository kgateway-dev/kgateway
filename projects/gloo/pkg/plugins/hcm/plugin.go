package hcm

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"
)

const (
	// filter info
	pluginStage = plugins.PostInAuth
)

func NewPlugin() *Plugin {
	return &Plugin{}
}

var _ plugins.Plugin = new(Plugin)
var _ plugins.ListenerPlugin = new(Plugin)

type Plugin struct {
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) ProcessListener(params plugins.Params, in *v1.Listener, out *envoyapi.Listener) error {
	hl, ok := in.ListenerType.(*v1.Listener_HttpListener)
	if !ok {
		return nil
	}
	if hl.HttpListener == nil {
		return nil
	}
	if hl.HttpListener.ListenerPlugins == nil {
		return nil
	}
	hcmSettings := hl.HttpListener.ListenerPlugins.HttpConnectionManagerSettings
	if hcmSettings == nil {
		return nil
	}
	for _, f := range out.FilterChains {
		for i, filter := range f.Filters {
			if filter.Name == envoyutil.HTTPConnectionManager {
				// get config
				var cfg envoyhttp.HttpConnectionManager
				err := translatorutil.ParseConfig(&filter, &cfg)
				// this should never error
				if err != nil {
					return err
				}

				copySettings(&cfg, hcmSettings)

				f.Filters[i], err = translatorutil.NewFilterWithConfig(envoyutil.HTTPConnectionManager, &cfg)
				// this should never error
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func copySettings(cfg *envoyhttp.HttpConnectionManager, hcmSettings *hcm.HttpConnectionManagerSettings) {
	cfg.UseRemoteAddress = hcmSettings.UseRemoteAddress
	cfg.XffNumTrustedHops = hcmSettings.XffNumTrustedHops
	cfg.SkipXffAppend = hcmSettings.SkipXffAppend
	cfg.Via = hcmSettings.Via
	cfg.GenerateRequestId = hcmSettings.GenerateRequestId
	cfg.Proxy_100Continue = hcmSettings.Proxy_100Continue
	cfg.StreamIdleTimeout = hcmSettings.StreamIdleTimeout
	cfg.IdleTimeout = hcmSettings.IdleTimeout
	cfg.MaxRequestHeadersKb = hcmSettings.MaxRequestHeadersKb
	cfg.RequestTimeout = hcmSettings.RequestTimeout
	cfg.DrainTimeout = hcmSettings.DrainTimeout
	cfg.DelayedCloseTimeout = hcmSettings.DelayedCloseTimeout
	cfg.ServerName = hcmSettings.ServerName
}
