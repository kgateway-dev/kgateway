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
	"github.com/rotisserie/eris"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"google.golang.org/protobuf/proto"

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

		var newAlsCfg envoyal.AccessLog
		var err error

		// Make the "base" config with output destination
		switch cfgType := al.GetOutputDestination().(type) {
		case *als.AccessLog_FileSink:
			var cfg envoyalfile.FileAccessLog
			if err = copyFileSettings(&cfg, cfgType); err != nil {
				return nil, err
			}

			if newAlsCfg, err = translatorutil.NewAccessLogWithConfig(wellknown.FileAccessLog, &cfg); err != nil {
				return nil, err
			}

		case *als.AccessLog_GrpcService:
			var cfg envoygrpc.HttpGrpcAccessLogConfig
			err := copyGrpcSettings(&cfg, cfgType)
			if err != nil {
				return nil, err
			}

			newAlsCfg, err = translatorutil.NewAccessLogWithConfig(wellknown.HTTPGRPCAccessLog, &cfg)
			if err != nil {
				return nil, err
			}
		}

		// Create and add the filter
		filter := al.GetFilter()
		err = translateFilter(&newAlsCfg, filter)
		if err != nil {
			return nil, err
		}

		results = append(results, &newAlsCfg)

	}

	logCfg = append(logCfg, results...)
	return logCfg, nil
}

// Since we are using the same proto def, marshal out of gloo format and unmarshal into envoy format
func translateFilter(accessLog *envoyal.AccessLog, inFilter *als.AccessLogFilter) error {
	if inFilter == nil {
		return nil
	}

	// We need to validate the enums in the filter manually because the protobuf libraries
	// do not validate them, for "compatibilty reasons". It's nicer to catch them here instead
	// of sending bad configs to Envoy.
	if err := validateFilterEnums(inFilter); err != nil {
		return err
	}

	bytes, err := proto.Marshal(inFilter)
	if err != nil {
		return err
	}

	outFilter := &envoyal.AccessLogFilter{}
	if err := proto.Unmarshal(bytes, outFilter); err != nil {
		return err
	}

	accessLog.Filter = outFilter
	return nil
}

var (
	InvalidEnumValueError = func(filterName string, fieldName string, value string) error {
		return eris.Errorf("Invalid value of %s in Enum field %s of %s", value, fieldName, filterName)
	}
	WrapInvalidEnumValueError = func(filterName string, err error) error {
		return eris.Wrap(err, fmt.Sprintf("Invalid subfilter in %s", filterName))
	}
)

func validateFilterEnums(filter *als.AccessLogFilter) error {
	switch filter := filter.GetFilterSpecifier().(type) {
	case *als.AccessLogFilter_RuntimeFilter:
		denominator := filter.RuntimeFilter.GetPercentSampled().GetDenominator()
		name := v3.FractionalPercent_DenominatorType_name[int32(denominator.Number())]
		if name == "" {
			return InvalidEnumValueError("RuntimeFilter", "FractionalPercent.Denominator", denominator.String())
		}
	case *als.AccessLogFilter_StatusCodeFilter:
		op := filter.StatusCodeFilter.GetComparison().GetOp()
		name := als.ComparisonFilter_Op_name[int32(op.Number())]
		if name == "" {
			return InvalidEnumValueError("StatusCodeFilter", "ComparisonFilter.Op", op.String())
		}
	case *als.AccessLogFilter_DurationFilter:
		op := filter.DurationFilter.GetComparison().GetOp()
		name := als.ComparisonFilter_Op_name[int32(op.Number())]
		if name == "" {
			return InvalidEnumValueError("DurationFilter", "ComparisonFilter.Op", op.String())
		}
	case *als.AccessLogFilter_AndFilter:
		subfilters := filter.AndFilter.GetFilters()
		for _, f := range subfilters {
			err := validateFilterEnums(f)
			if err != nil {
				return WrapInvalidEnumValueError("AndFilter", err)
			}
		}
	case *als.AccessLogFilter_OrFilter:
		subfilters := filter.OrFilter.GetFilters()
		for _, f := range subfilters {
			err := validateFilterEnums(f)
			if err != nil {
				return WrapInvalidEnumValueError("OrFilter", err)
			}
		}
	case *als.AccessLogFilter_GrpcStatusFilter:
		statuses := filter.GrpcStatusFilter.GetStatuses()
		for _, status := range statuses {
			name := als.GrpcStatusFilter_Status_name[int32(status.Number())]
			if name == "" {
				return InvalidEnumValueError("GrpcStatusFilter", "Status", status.String())
			}
		}

	}

	return nil
}

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
