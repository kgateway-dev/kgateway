package als

import (
	"fmt"

	envoyal "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyalfile "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	envoygrpc "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_req_without_query "github.com/envoyproxy/go-control-plane/envoy/extensions/formatter/req_without_query/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/proto"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"google.golang.org/protobuf/runtime/protoiface"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/als"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

var (
	_ plugins.Plugin                      = new(plugin)
	_ plugins.HttpConnectionManagerPlugin = new(plugin)
)

const (
	ExtensionName = "als"
	ClusterName   = "access_log_cluster"
)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) {
}

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
	return err
}

// The AccessLogging plugin configures access logging for envoy, regardless of whether it will be applied to
// an HttpConnectionManager or TcpProxy NetworkFilter. We have exposed HttpConnectionManagerPlugins to enable
// fine grained configuration of the HCM across multiple plugins. However, the TCP proxy is still configured
// by the TCP plugin only. To keep our access logging translation in a single place, we expose this function
// and the Tcp plugin calls out to it.
func ProcessAccessLogPlugins(service *als.AccessLoggingService, logCfg []*envoyal.AccessLog) ([]*envoyal.AccessLog, error) {
	results := make([]*envoyal.AccessLog, 0, len(service.GetAccessLog()))
	for _, al := range service.GetAccessLog() {

		var config protoiface.MessageV1 // als.isAccessLog_OutputDestination?
		var name string
		cfg0 := al.GetOutputDestination()
		// cfgType0 := reflect.TypeOf(cfg0).String()
		// fmt.Printf("%s", cfgType0)
		// Process OutputDestination
		switch cfgType := cfg0.(type) {
		case *als.AccessLog_FileSink:
			var cfg envoyalfile.FileAccessLog
			err := copyFileSettings(&cfg, cfgType)
			if err != nil {
				return nil, err
			}
			config = &cfg
			name = wellknown.FileAccessLog

		case *als.AccessLog_GrpcService:
			var cfg envoygrpc.HttpGrpcAccessLogConfig
			err := copyGrpcSettings(&cfg, cfgType)
			if err != nil {
				return nil, err
			}
			config = &cfg
			name = wellknown.HTTPGRPCAccessLog
		}

		newCfg, err := translatorutil.NewAccessLogWithConfig(name, config)
		if err != nil {
			return nil, err
		}

		//accessLogFilter = protoiface.MessageV1
		//accessLogFilter := copyFilter(al)
		//err := copyFilterSettings()
		// Process AccessLogFilter
		//var outFilter protoiface.MessageV1
		// isThereAFilter := al.GetFilter()

		// if isThereAFilter != nil {
		// 	switch filter := isThereAFilter.FilterSpecifier.(type) {
		// 	case *als.AccessLogFilter_TraceableFilter:
		// 		outFilter := translateTraceableFilter(filter)
		// 		newCfg.Filter = &envoyal.AccessLogFilter{FilterSpecifier: &envoyal.AccessLogFilter_TraceableFilter{TraceableFilter: outFilter}}
		// 	}
		// }

		//filter := al.GetFilter()
		filter := &als.AccessLogFilter{FilterSpecifier: &als.AccessLogFilter_TraceableFilter{}}

		fmt.Printf("In Filter %+v", filter)
		envoyFilter, err := translateFilter(filter)
		if err != nil {
			fmt.Printf("Error %s", err)
			return nil, err
		}
		fmt.Printf("Out Filter %+v", envoyFilter)
		newCfg.Filter = envoyFilter

		//newCfg.Filter = &envoyal.AccessLogFilter{FilterSpecifier: &envoyal.AccessLogFilter_TraceableFilter{}}
		//filter = copyFilter(al)

		results = append(results, &newCfg)

	}
	logCfg = append(logCfg, results...)
	return logCfg, nil
}

func translateFilter(inFilter *als.AccessLogFilter) (*envoyal.AccessLogFilter, error) {
	if inFilter == nil {
		return nil, nil
	}
	bytes, err := proto.Marshal(inFilter)
	if err != nil {
		return nil, err
	}

	outFilter := &envoyal.AccessLogFilter{}
	if err := proto.Unmarshal(bytes, outFilter); err != nil {
		return nil, err
	}
	return outFilter, nil
}

// func ToRateLimiterResourceSpec_Raw(in *solo_apis_rl.RateLimitConfigSpec_Raw) (*rate_limiter_types.RateLimitConfigSpec_Raw, error) {
// 	if in == nil {
// 		return nil, nil
// 	}

// 	bytes, err := proto.Marshal(in)
// 	if err != nil {
// 		return nil, err
// 	}

// 	out := &rate_limiter_types.RateLimitConfigSpec_Raw{}
// 	if err := proto.Unmarshal(bytes, out); err != nil {
// 		return nil, err
// 	}
// 	return out, nil
// }

func copyGrpcSettings(cfg *envoygrpc.HttpGrpcAccessLogConfig, alsSettings *als.AccessLog_GrpcService) error {
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
	cfg.AdditionalRequestHeadersToLog = alsSettings.GrpcService.GetAdditionalRequestHeadersToLog()
	cfg.AdditionalResponseHeadersToLog = alsSettings.GrpcService.GetAdditionalResponseHeadersToLog()
	cfg.AdditionalResponseTrailersToLog = alsSettings.GrpcService.GetAdditionalResponseTrailersToLog()
	cfg.CommonConfig = &envoygrpc.CommonGrpcAccessLogConfig{
		LogName:             alsSettings.GrpcService.GetLogName(),
		GrpcService:         svc,
		TransportApiVersion: envoycore.ApiVersion_V3,
	}
	return cfg.Validate()
}

func copyFileSettings(cfg *envoyalfile.FileAccessLog, alsSettings *als.AccessLog_FileSink) error {
	cfg.Path = alsSettings.FileSink.GetPath()

	query := &envoy_req_without_query.ReqWithoutQuery{}
	typedConfig, err := utils.MessageToAny(query)
	if err != nil {
		return err
	}
	formatterExtensions := []*envoycore.TypedExtensionConfig{
		{
			Name:        "envoy.formatter.req_without_query",
			TypedConfig: typedConfig,
		},
	}

	switch fileSinkType := alsSettings.FileSink.GetOutputFormat().(type) {
	case *als.FileSink_StringFormat:
		if fileSinkType.StringFormat != "" {
			cfg.AccessLogFormat = &envoyalfile.FileAccessLog_LogFormat{
				LogFormat: &envoycore.SubstitutionFormatString{
					Format: &envoycore.SubstitutionFormatString_TextFormat{
						TextFormat: fileSinkType.StringFormat,
					},
					Formatters: formatterExtensions,
				},
			}
		}
	case *als.FileSink_JsonFormat:
		cfg.AccessLogFormat = &envoyalfile.FileAccessLog_LogFormat{
			LogFormat: &envoycore.SubstitutionFormatString{
				Format: &envoycore.SubstitutionFormatString_JsonFormat{
					JsonFormat: fileSinkType.JsonFormat,
				},
				Formatters: formatterExtensions,
			},
		}
	}
	return cfg.Validate()
}

// func copyFilter(cfg *envoyal.AccessLogFilter, accessLog *als.AccessLog) error {
// 	var config protoiface.MessageV1 // als.isAccessLog_OutputDestination?
// 	var name string
// 	cfg0 := al.GetOutputDestination()
// 	// cfgType0 := reflect.TypeOf(cfg0).String()
// 	// fmt.Printf("%s", cfgType0)
// }

func translateStatusCodeFilter(inFilter *als.StatusCodeFilter) *envoyal.StatusCodeFilter {
	if inFilter == nil {
		return nil
	}

	return &envoyal.StatusCodeFilter{
		Comparison: translateComparisonFilter(inFilter.GetComparison()),
	}
}

func translateDurationFilter(inFilter *als.DurationFilter) *envoyal.DurationFilter {
	if inFilter == nil {
		return nil
	}

	return &envoyal.DurationFilter{
		Comparison: translateComparisonFilter(inFilter.GetComparison()),
	}
}

func translateNotHealthCheckFilter(inFilter *als.NotHealthCheckFilter) *envoyal.NotHealthCheckFilter {
	if inFilter == nil {
		return nil
	}

	return &envoyal.NotHealthCheckFilter{}
}

func translateTraceableFilter(inFilter *als.AccessLogFilter_TraceableFilter) *envoyal.TraceableFilter {
	if inFilter == nil {
		return nil
	}

	return &envoyal.TraceableFilter{}
}

func translateRuntimeFilter(inFilter *als.RuntimeFilter) *envoyal.RuntimeFilter {
	if inFilter == nil {
		return nil
	}

	return &envoyal.RuntimeFilter{
		RuntimeKey: inFilter.GetRuntimeKey(),
		//PercentSampled:           inFilter.GetPercentSampled(),
		UseIndependentRandomness: inFilter.GetUseIndependentRandomness(),
	}
}

func translateAndFilter(inFilter *als.AndFilter) *envoyal.AndFilter {
	if inFilter == nil {
		return nil
	}

	return &envoyal.AndFilter{}
}

func translateHeaderFilter(inFilter *als.HeaderFilter) *envoyal.HeaderFilter {
	if inFilter == nil {
		return nil
	}

	return &envoyal.HeaderFilter{}
}

func translateResponseFlagFilter(inFilter *als.ResponseFlagFilter) *envoyal.ResponseFlagFilter {
	if inFilter == nil {
		return nil
	}

	return &envoyal.ResponseFlagFilter{}
}

func translateGrpcStatusFilter(inFilter *als.GrpcStatusFilter) *envoyal.GrpcStatusFilter {
	if inFilter == nil {
		return nil
	}

	return &envoyal.GrpcStatusFilter{}
}

func translateComparisonFilter(inFilter *als.ComparisonFilter) *envoyal.ComparisonFilter {
	if inFilter == nil {
		return nil
	}

	glooToEnvoyOpMap := map[als.ComparisonFilter_Op]envoyal.ComparisonFilter_Op{
		als.ComparisonFilter_EQ: envoyal.ComparisonFilter_EQ,
		als.ComparisonFilter_GE: envoyal.ComparisonFilter_GE,
		als.ComparisonFilter_LE: envoyal.ComparisonFilter_LE,
	}

	return &envoyal.ComparisonFilter{
		Op: glooToEnvoyOpMap[inFilter.GetOp()],
		//Value: inFilter.GetValue(),
	}
}

// func translateFractionalPercent(fraction *solo.io.envoy.type.v3.FractionalPercent) *envoytype.FractionalPercent{
// 	if inFilter == nil {
// 		return nil
// 	}

// 	return &envoytype.FractionalPercent{

// 	}
// }
