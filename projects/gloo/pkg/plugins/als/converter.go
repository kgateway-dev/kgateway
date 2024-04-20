package als

import (
	"errors"
	"fmt"
	"strings"

	envoyal "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyalfile "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	envoygrpc "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3"
	envoy_metadata_formatter "github.com/envoyproxy/go-control-plane/envoy/extensions/formatter/metadata/v3"
	envoy_req_without_query "github.com/envoyproxy/go-control-plane/envoy/extensions/formatter/req_without_query/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/rotisserie/eris"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/v3"
	"google.golang.org/protobuf/proto"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/als"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

var (
	NoValueError = func(filterName string, fieldName string) error {
		return eris.Errorf("No value found for field %s of %s", fieldName, filterName)
	}
	InvalidEnumValueError = func(filterName string, fieldName string, value string) error {
		return eris.Errorf("Invalid value of %s in Enum field %s of %s", value, fieldName, filterName)
	}
	WrapInvalidEnumValueError = func(filterName string, err error) error {
		return eris.Wrap(err, fmt.Sprintf("Invalid subfilter in %s", filterName))
	}
)

type aclType string

var (
	Hcm          aclType = "hcm"
	HttpListener aclType = "http-listener"
	Tcp          aclType = "tcp"
	Udp          aclType = "udp"
)

// DetectUnusefulCmds will detect commands that are not useful in the current configuration
// It returns errors that may some day be bubbled up arbitrarly high.
// See https://github.com/envoyproxy/envoy/blob/313b6fb7cf0f806e74a2d42c93e7c1fcccce2391/docs/root/configuration/observability/access_log/usage.rst?plain=1#L114-L123
func DetectUnusefulCmds(filterLocationType aclType, proposedLogFormats []*envoyal.AccessLog) error {

	// TODO: programatically make sure we cover all command operators as found
	// in https://github.com/envoyproxy/envoy/blob/0f3e4aa373db6bbb7643b1bb60b0cb60d5b39df8/source/common/formatter/stream_info_formatter.cc#L1443
	// This could take place in ci especially if envoy version has been changed.

	var unusefulCmds []string
	switch filterLocationType {
	case Hcm:
		unusefulCmds = []string{"DOWNSTREAM_TRANSPORT_FAILURE_REASON"}
	case HttpListener:
	case Tcp:
		unusefulCmds = []string{"DOWNSTREAM_TRANSPORT_FAILURE_REASON", "REQ", "RESP", "TRAILER", "METADATA"}
	case Udp:
		unusefulCmds = []string{"UPSTREAM_TRANSPORT_FAILURE_REASON", "DOWNSTREAM_TRANSPORT_FAILURE_REASON", "REQ", "RESP", "TRAILER"}
	default:
		return errors.New("unknown accesslog level, cannot detect unuseful commands")
	}

	// TODO: Warn against deprecated commands for future proofing
	// deprecatedCmds := []struct{ cur, new string }{{cur: "DYNAMIC_METADATA", new: "METADATA"}}

	var errs []error
	for _, plf := range proposedLogFormats {
		// Dont put command operators in Names
		// Therefore we can extract their usage via strings
		dumpedACLSTR := fmt.Sprintf("%v", plf)
		issues := []string{}
		for _, cmd := range unusefulCmds {
			if strings.Contains(dumpedACLSTR, cmd) {
				issues = append(issues, cmd)
			}
		}
		// For a given proposed format this will include all the detected bad operators.
		if len(issues) > 0 {
			errs = append(errs, fmt.Errorf("unuseful command operators found in access log %s: %v", plf.GetName(), issues))
		}

	}

	return errors.Join(errs...)
}

// ProcessAccessLogPlugins will configure access logging for envoy, regardless of whether it will be applied to
// an HttpConnectionManager, http listener, TcpProxy NetworkFilter or perhaps someday a UdpProxy NetworkFilter.
// We have exposed plugins to allow configuration of http listeners and filters across multiple plugins.
// However, the TCP proxy is still configured by the TCP plugin only.
// To keep our access logging translation in a single place, we expose this function
// and the TCP plugin calls out to it.
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

func validateFilterEnums(filter *als.AccessLogFilter) error {
	switch filter := filter.GetFilterSpecifier().(type) {
	case *als.AccessLogFilter_RuntimeFilter:
		denominator := filter.RuntimeFilter.GetPercentSampled().GetDenominator()
		name := v3.FractionalPercent_DenominatorType_name[int32(denominator.Number())]
		if name == "" {
			return InvalidEnumValueError("RuntimeFilter", "FractionalPercent.Denominator", denominator.String())
		}
		runtimeKey := filter.RuntimeFilter.GetRuntimeKey()
		if len(runtimeKey) == 0 {
			return NoValueError("RuntimeFilter", "FractionalPercent.RuntimeKey")
		}
	case *als.AccessLogFilter_StatusCodeFilter:
		op := filter.StatusCodeFilter.GetComparison().GetOp()
		name := als.ComparisonFilter_Op_name[int32(op.Number())]
		if name == "" {
			return InvalidEnumValueError("StatusCodeFilter", "ComparisonFilter.Op", op.String())
		}
		value := filter.StatusCodeFilter.GetComparison().GetValue()
		if value == nil {
			return NoValueError("StatusCodeFilter", "ComparisonFilter.Value")
		}
		if len(value.GetRuntimeKey()) == 0 {
			return NoValueError("StatusCodeFilter", "ComparisonFilter.Value.RuntimeKey")
		}
	case *als.AccessLogFilter_DurationFilter:
		op := filter.DurationFilter.GetComparison().GetOp()
		name := als.ComparisonFilter_Op_name[int32(op.Number())]
		if name == "" {
			return InvalidEnumValueError("DurationFilter", "ComparisonFilter.Op", op.String())
		}
		value := filter.DurationFilter.GetComparison().GetValue()
		if value == nil {
			return NoValueError("DurationFilter", "ComparisonFilter.Value")
		}
		if len(value.GetRuntimeKey()) == 0 {
			return NoValueError("DurationFilter", "ComparisonFilter.Value.RuntimeKey")
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

	formatterExtensions, err := getFormatterExtensions()
	if err != nil {
		return err
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

func getFormatterExtensions() ([]*envoycore.TypedExtensionConfig, error) {
	reqWithoutQueryFormatter := &envoy_req_without_query.ReqWithoutQuery{}
	reqWithoutQueryFormatterTc, err := utils.MessageToAny(reqWithoutQueryFormatter)
	if err != nil {
		return nil, err
	}

	mdFormatter := &envoy_metadata_formatter.Metadata{}
	mdFormatterTc, err := utils.MessageToAny(mdFormatter)
	if err != nil {
		return nil, err
	}

	return []*envoycore.TypedExtensionConfig{
		{
			Name:        "envoy.formatter.req_without_query",
			TypedConfig: reqWithoutQueryFormatterTc,
		},
		{
			Name:        "envoy.formatter.metadata",
			TypedConfig: mdFormatterTc,
		},
	}, nil

}
