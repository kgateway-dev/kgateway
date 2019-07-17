package als

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyalcfg "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v2"
	envoyal "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/als"
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
	alSettings := in.GetPlugins()
	if alSettings.Als == nil {
		return nil
	}
	// Use a switch here to support TCP in the future
	switch listenerType := in.GetListenerType().(type) {
	case *v1.Listener_HttpListener:
		if listenerType.HttpListener == nil {
			return nil
		}
		if listenerType.HttpListener.ListenerPlugins == nil {
			return nil
		}
		for _, f := range out.FilterChains {
			for i, filter := range f.Filters {
				if filter.Name == envoyutil.HTTPConnectionManager {
					// get config
					var hcmCfg envoyhttp.HttpConnectionManager
					err := translatorutil.ParseConfig(&filter, &hcmCfg)
					// this should never error
					if err != nil {
						return err
					}

					accessLogs := hcmCfg.GetAccessLog()
					hcmCfg.AccessLog = handleAccessLogPlugins(alSettings.Als, accessLogs)


					f.Filters[i], err = translatorutil.NewFilterWithConfig(envoyutil.HTTPConnectionManager, &hcmCfg)
					// this should never error
					if err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func handleAccessLogPlugins(service *als.AccessLoggingService, logCfg []*envoyal.AccessLog) []*envoyal.AccessLog {
	for _, al := range service.GetAccessLog() {
		switch cfgType := al.GetOutputDestination().(type) {
		case *als.AccessLog_FileSink:
			var cfg envoyalcfg.FileAccessLog

		}
	}
	return logCfg
}

func copyFileSettings(cfg *envoyalcfg.FileAccessLog, alsSettings *als.AccessLog_FileSink) {

}
