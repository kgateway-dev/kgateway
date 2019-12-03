package als

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyalcfg "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v2"
	envoyal "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	"github.com/solo-io/gloo/pkg/utils/protoutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/als"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/go-utils/errors"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/util"
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

func (p *Plugin) ProcessListener(params plugins.Params, in *v1.Listener, out *envoyapi.Listener) error {
	if in.GetOptions() == nil {
		return nil
	}
	alSettings := in.GetOptions()
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
				if filter.Name == util.HTTPConnectionManager {
					// get config
					var hcmCfg envoyhttp.HttpConnectionManager
					err := translatorutil.ParseConfig(filter, &hcmCfg)
					// this should never error
					if err != nil {
						return err
					}

					accessLogs := hcmCfg.GetAccessLog()
					hcmCfg.AccessLog, err = handleAccessLogPlugins(alSettings.AccessLoggingService, accessLogs, params)
					if err != nil {
						return err
					}

					f.Filters[i], err = translatorutil.NewFilterWithConfig(util.HTTPConnectionManager, &hcmCfg)
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
				if filter.Name == util.TCPProxy {
					// get config
					var tcpCfg envoytcp.TcpProxy
					err := translatorutil.ParseConfig(filter, &tcpCfg)
					// this should never error
					if err != nil {
						return err
					}

					accessLogs := tcpCfg.GetAccessLog()
					tcpCfg.AccessLog, err = handleAccessLogPlugins(alSettings.AccessLoggingService, accessLogs, params)
					if err != nil {
						return err
					}

					f.Filters[i], err = translatorutil.NewFilterWithConfig(util.TCPProxy, &tcpCfg)
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
			var cfg envoyalcfg.FileAccessLog
			if err := copyFileSettings(&cfg, cfgType); err != nil {
				return nil, err
			}
			newAlsCfg, err := translatorutil.NewAccessLogWithConfig(util.FileAccessLog, &cfg)
			if err != nil {
				return nil, err
			}
			results = append(results, &newAlsCfg)
		case *als.AccessLog_GrpcService:
			var cfg envoyalcfg.HttpGrpcAccessLogConfig
			if err := copyGrpcSettings(&cfg, cfgType, params); err != nil {
				return nil, err
			}
			newAlsCfg, err := translatorutil.NewAccessLogWithConfig(util.HTTPGRPCAccessLog, &cfg)
			if err != nil {
				return nil, err
			}
			results = append(results, &newAlsCfg)
		}
	}
	logCfg = append(logCfg, results...)
	return logCfg, nil
}

func copyGrpcSettings(cfg *envoyalcfg.HttpGrpcAccessLogConfig, alsSettings *als.AccessLog_GrpcService, params plugins.Params) error {
	if alsSettings.GrpcService == nil {
		return errors.New("grpc service object cannot be nil")
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
	cfg.CommonConfig = &envoyalcfg.CommonGrpcAccessLogConfig{
		LogName:     alsSettings.GrpcService.LogName,
		GrpcService: svc,
	}
	return cfg.Validate()
}

func copyFileSettings(cfg *envoyalcfg.FileAccessLog, alsSettings *als.AccessLog_FileSink) error {
	cfg.Path = alsSettings.FileSink.Path
	switch fileSinkType := alsSettings.FileSink.GetOutputFormat().(type) {
	case *als.FileSink_StringFormat:
		cfg.AccessLogFormat = &envoyalcfg.FileAccessLog_Format{
			Format: fileSinkType.StringFormat,
		}
	case *als.FileSink_JsonFormat:
		converted, err := protoutils.StructGogoToPb(fileSinkType.JsonFormat)
		if err != nil {
			return err
		}
		cfg.AccessLogFormat = &envoyalcfg.FileAccessLog_JsonFormat{
			JsonFormat: converted,
		}
	}
	return cfg.Validate()
}
