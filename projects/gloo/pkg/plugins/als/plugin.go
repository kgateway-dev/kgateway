package als

import (
	envoyal "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyalfile "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	envoygrpc "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/als"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"
)

const (
	ClusterName = "access_log_cluster"
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

func (p *Plugin) ProcessListener(params plugins.Params, in *v1.Listener, out *envoy_config_listener_v3.Listener) error {
	if in.GetOptions() == nil {
		return nil
	}
	alSettings := in.GetOptions()
	if alSettings.GetAccessLoggingService() == nil {
		return nil
	}
	switch listenerType := in.GetListenerType().(type) {
	case *v1.Listener_HttpListener:
		if listenerType.HttpListener == nil {
			return nil
		}
		for _, f := range out.GetFilterChains() {
			for i, filter := range f.GetFilters() {
				if filter.GetName() == wellknown.HTTPConnectionManager {
					// get config
					var hcmCfg envoyhttp.HttpConnectionManager
					err := translatorutil.ParseTypedConfig(filter, &hcmCfg)
					// this should never error
					if err != nil {
						return err
					}

					accessLogs := hcmCfg.GetAccessLog()
					hcmCfg.AccessLog, err = handleAccessLogPlugins(alSettings.GetAccessLoggingService(), accessLogs, params)
					if err != nil {
						return err
					}

					f.GetFilters()[i], err = translatorutil.NewFilterWithTypedConfig(wellknown.HTTPConnectionManager, &hcmCfg)
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
		for _, f := range out.GetFilterChains() {
			for i, filter := range f.GetFilters() {
				if filter.GetName() == wellknown.TCPProxy {
					// get config
					var tcpCfg envoytcp.TcpProxy
					err := translatorutil.ParseTypedConfig(filter, &tcpCfg)
					// this should never error
					if err != nil {
						return err
					}

					accessLogs := tcpCfg.GetAccessLog()
					tcpCfg.AccessLog, err = handleAccessLogPlugins(alSettings.GetAccessLoggingService(), accessLogs, params)
					if err != nil {
						return err
					}

					f.GetFilters()[i], err = translatorutil.NewFilterWithTypedConfig(wellknown.TCPProxy, &tcpCfg)
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

func handleAccessLogPlugins(service *als.AccessLoggingService, logCfg []*envoyal.AccessLog, params plugins.Params) ([]*envoyal.AccessLog, error) {
	results := make([]*envoyal.AccessLog, 0, len(service.GetAccessLog()))
	for _, al := range service.GetAccessLog() {
		switch cfgType := al.GetOutputDestination().(type) {
		case *als.AccessLog_FileSink:
			var cfg envoyalfile.FileAccessLog
			if err := copyFileSettings(&cfg, cfgType); err != nil {
				return nil, err
			}
			newAlsCfg, err := translatorutil.NewAccessLogWithConfig(wellknown.FileAccessLog, &cfg)
			if err != nil {
				return nil, err
			}
			results = append(results, &newAlsCfg)
		case *als.AccessLog_GrpcService:
			var cfg envoygrpc.HttpGrpcAccessLogConfig
			if err := copyGrpcSettings(&cfg, cfgType, params); err != nil {
				return nil, err
			}
			newAlsCfg, err := translatorutil.NewAccessLogWithConfig(wellknown.HTTPGRPCAccessLog, &cfg)
			if err != nil {
				return nil, err
			}
			results = append(results, &newAlsCfg)
		}
	}
	logCfg = append(logCfg, results...)
	return logCfg, nil
}

func copyGrpcSettings(cfg *envoygrpc.HttpGrpcAccessLogConfig, alsSettings *als.AccessLog_GrpcService, params plugins.Params) error {
	if alsSettings.GrpcService == nil {
		return eris.New("grpc service object cannot be nil")
	}

	svc := &envoycore.GrpcService{
		TargetSpecifier: &envoycore.GrpcService_EnvoyGrpc_{
			EnvoyGrpc: &envoycore.GrpcService_EnvoyGrpc{
				ClusterName: alsSettings.GrpcService.GetStaticClusterName(),
			},
		},
	}
	cfg.AdditionalRequestHeadersToLog = alsSettings.GrpcService.AdditionalRequestHeadersToLog
	cfg.AdditionalResponseHeadersToLog = alsSettings.GrpcService.AdditionalResponseHeadersToLog
	cfg.AdditionalResponseTrailersToLog = alsSettings.GrpcService.AdditionalResponseTrailersToLog
	cfg.CommonConfig = &envoygrpc.CommonGrpcAccessLogConfig{
		LogName:             alsSettings.GrpcService.GetLogName(),
		GrpcService:         svc,
		TransportApiVersion: envoycore.ApiVersion_V3,
	}
	return cfg.Validate()
}

func copyFileSettings(cfg *envoyalfile.FileAccessLog, alsSettings *als.AccessLog_FileSink) error {
	cfg.Path = alsSettings.FileSink.Path
	switch fileSinkType := alsSettings.FileSink.GetOutputFormat().(type) {
	case *als.FileSink_StringFormat:
		if fileSinkType.StringFormat != "" {
			cfg.AccessLogFormat = &envoyalfile.FileAccessLog_LogFormat{
				LogFormat: &envoycore.SubstitutionFormatString{
					Format: &envoycore.SubstitutionFormatString_TextFormat{
						TextFormat: fileSinkType.StringFormat,
					},
				},
			}
		}
	case *als.FileSink_JsonFormat:
		cfg.AccessLogFormat = &envoyalfile.FileAccessLog_LogFormat{
			LogFormat: &envoycore.SubstitutionFormatString{
				Format: &envoycore.SubstitutionFormatString_JsonFormat{
					JsonFormat: fileSinkType.JsonFormat,
				},
			},
		}
	}
	return cfg.Validate()
}
