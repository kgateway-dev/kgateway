package als

import (
	"context"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyalcfg "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v2"
	envoyal "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
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
	if in.GetPlugins() == nil {
		return nil
	}
	alSettings := in.GetPlugins()
	if alSettings.AccessLoggingService == nil {
		return nil
	}
	switch listenerType := in.GetListenerType().(type) {
	case *v1.Listener_HttpListener:
		if listenerType.HttpListener == nil {
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
					hcmCfg.AccessLog, err = handleAccessLogPlugins(params.Ctx, alSettings.AccessLoggingService, accessLogs)
					if err != nil {
						return err
					}

					f.Filters[i], err = translatorutil.NewFilterWithConfig(envoyutil.HTTPConnectionManager, &hcmCfg)
					// this should never error
					if err != nil {
						return err
					}
				}
			}
		}
	case *v1.Listener_TcpListener:
		if listenerType.TcpListener == nil {
			return nil
		}
		for _, f := range out.FilterChains {
			for i, filter := range f.Filters {
				if filter.Name == envoyutil.TCPProxy {
					// get config
					var tcpCfg envoytcp.TcpProxy
					err := translatorutil.ParseConfig(&filter, &tcpCfg)
					// this should never error
					if err != nil {
						return err
					}

					accessLogs := tcpCfg.GetAccessLog()
					tcpCfg.AccessLog, err = handleAccessLogPlugins(params.Ctx, alSettings.AccessLoggingService, accessLogs)
					if err != nil {
						return err
					}

					f.Filters[i], err = translatorutil.NewFilterWithConfig(envoyutil.TCPProxy, &tcpCfg)
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

func handleAccessLogPlugins(ctx context.Context, service *als.AccessLoggingService, logCfg []*envoyal.AccessLog) ([]*envoyal.AccessLog, error) {
	results := make([]*envoyal.AccessLog, 0, len(service.GetAccessLog()))
	for _, al := range service.GetAccessLog() {
		switch cfgType := al.GetOutputDestination().(type) {
		case *als.AccessLog_FileSink:
			var cfg envoyalcfg.FileAccessLog
			copyFileSettings(&cfg, cfgType)
			newAlsCfg, err := translatorutil.NewAccessLogWithConfig(&cfg)
			if err != nil {
				return nil, err
			}
			results = append(results, &newAlsCfg)
		}
	}
	logCfg = append(logCfg, results...)
	return logCfg, nil
}

func copyFileSettings(cfg *envoyalcfg.FileAccessLog, alsSettings *als.AccessLog_FileSink) {
	cfg.Path = alsSettings.FileSink.Path
	switch fileSinkType := alsSettings.FileSink.GetOutputFormat().(type) {
	case *als.FileSink_StringFormat:
		cfg.AccessLogFormat = &envoyalcfg.FileAccessLog_Format{
			Format: fileSinkType.StringFormat,
		}
	case *als.FileSink_JsonFormat:
		cfg.AccessLogFormat = &envoyalcfg.FileAccessLog_JsonFormat{
			JsonFormat: fileSinkType.JsonFormat,
		}
	}
}
