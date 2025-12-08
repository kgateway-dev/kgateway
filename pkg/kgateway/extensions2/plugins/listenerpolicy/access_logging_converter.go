package listenerpolicy

import (
	"encoding/json"
	"errors"
	"fmt"

	envoyaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyalfile "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	cel "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/filters/cel/v3"
	envoygrpc "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3"
	envoy_open_telemetry "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/open_telemetry/v3"
	envoy_metadata_formatter "github.com/envoyproxy/go-control-plane/envoy/extensions/formatter/metadata/v3"
	envoy_req_without_query "github.com/envoyproxy/go-control-plane/envoy/extensions/formatter/req_without_query/v3"
	envoymatcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	otelv1 "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	kwellknown "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

var ErrUnresolvedBackendRef = errors.New("unresolved backend reference")

const serviceNameKey = "service.name"

// convertAccessLogConfig transforms a list of AccessLog configurations into Envoy AccessLog configurations
// These access log configs can be either FileAccessLog, HttpGrpcAccessLogConfig or OpenTelemetryAccessLogConfig.
// The default service name needs to be set to the cluster name in the OpenTelemetryAccessLogConfig.
// Since the cluster name can only be determined during translation (when the specific gateway is passed),
// we return partially translated configs. As these configs are of different types, we return an list of interfaces
// that is stored in the IR to be fully translated during translation.
func convertAccessLogConfig(
	policy *kgateway.HTTPSettings,
	commoncol *collections.CommonCollections,
	krtctx krt.HandlerContext,
	parentSrc ir.ObjectSource,
) ([]proto.Message, error) {
	configs := policy.AccessLog

	if configs != nil && len(configs) == 0 {
		return nil, nil
	}

	grpcBackends := make(map[string]*ir.BackendObjectIR, len(policy.AccessLog))
	for idx, log := range configs {
		if log.GrpcService != nil {
			backend, err := commoncol.BackendIndex.GetBackendFromRef(krtctx, parentSrc, log.GrpcService.BackendRef.BackendObjectReference)
			// TODO: what is the correct behavior? maybe route to static blackhole?
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrUnresolvedBackendRef, err)
			}
			grpcBackends[getLogId(log.GrpcService.LogName, idx)] = backend
			continue
		}
		if log.OpenTelemetry != nil {
			backend, err := commoncol.BackendIndex.GetBackendFromRef(krtctx, parentSrc, log.OpenTelemetry.GrpcService.BackendRef.BackendObjectReference)
			// TODO: what is the correct behavior? maybe route to static blackhole?
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrUnresolvedBackendRef, err)
			}
			grpcBackends[getLogId(log.OpenTelemetry.GrpcService.LogName, idx)] = backend
		}
	}

	return translateAccessLogs(configs, grpcBackends)
}

func getLogId(logName string, idx int) string {
	return fmt.Sprintf("%s-%d", logName, idx)
}

func translateAccessLogs(configs []kgateway.AccessLog, grpcBackends map[string]*ir.BackendObjectIR) ([]proto.Message, error) {
	var results []proto.Message

	for idx, logConfig := range configs {
		accessLogCfg, err := translateAccessLog(logConfig, grpcBackends, idx)
		if err != nil {
			return nil, err
		}
		results = append(results, accessLogCfg)
	}

	return results, nil
}

// translateAccessLog creates an Envoy AccessLog configuration for a single log config
func translateAccessLog(logConfig kgateway.AccessLog, grpcBackends map[string]*ir.BackendObjectIR, accessLogId int) (proto.Message, error) {
	// Validate mutual exclusivity of sink types
	if logConfig.FileSink != nil && logConfig.GrpcService != nil {
		return nil, errors.New("access log config cannot have both file sink and grpc service")
	}

	var (
		accessLogCfg proto.Message
		err          error
	)

	switch {
	case logConfig.FileSink != nil:
		accessLogCfg, err = createFileAccessLog(logConfig.FileSink)
	case logConfig.GrpcService != nil:
		accessLogCfg, err = createGrpcAccessLog(logConfig.GrpcService, grpcBackends, accessLogId)
	case logConfig.OpenTelemetry != nil:
		accessLogCfg, err = createOTelAccessLog(logConfig.OpenTelemetry, grpcBackends, accessLogId)
	default:
		return nil, errors.New("no access log sink specified")
	}

	if err != nil {
		return nil, err
	}

	return accessLogCfg, nil
}

// createFileAccessLog generates a file-based access log configuration
func createFileAccessLog(fileSink *kgateway.FileSink) (proto.Message, error) {
	fileCfg := &envoyalfile.FileAccessLog{Path: fileSink.Path}

	formatterExtensions, err := getFormatterExtensions()
	if err != nil {
		return nil, err
	}

	switch {
	case fileSink.StringFormat != nil:
		fileCfg.AccessLogFormat = &envoyalfile.FileAccessLog_LogFormat{
			LogFormat: &envoycorev3.SubstitutionFormatString{
				Format: &envoycorev3.SubstitutionFormatString_TextFormatSource{
					TextFormatSource: &envoycorev3.DataSource{
						Specifier: &envoycorev3.DataSource_InlineString{
							InlineString: *fileSink.StringFormat,
						},
					},
				},
				Formatters: formatterExtensions,
			},
		}
	case fileSink.JsonFormat != nil:
		jsonStruct, err := convertJsonFormat(fileSink.JsonFormat)
		if err != nil {
			return nil, err
		}
		fileCfg.AccessLogFormat = &envoyalfile.FileAccessLog_LogFormat{
			LogFormat: &envoycorev3.SubstitutionFormatString{
				Format: &envoycorev3.SubstitutionFormatString_JsonFormat{
					JsonFormat: jsonStruct,
				},
				Formatters: formatterExtensions,
			},
		}
	}
	return fileCfg, nil
}

// createGrpcAccessLog generates a gRPC-based access log configuration
func createGrpcAccessLog(grpcService *kgateway.AccessLogGrpcService, grpcBackends map[string]*ir.BackendObjectIR, accessLogId int) (proto.Message, error) {
	var cfg envoygrpc.HttpGrpcAccessLogConfig
	if err := copyGrpcSettings(&cfg, grpcService, grpcBackends, accessLogId); err != nil {
		return nil, fmt.Errorf("error converting grpc access log config: %w", err)
	}
	return &cfg, nil
}

// createOTelAccessLog generates an OTel access log configuration
func createOTelAccessLog(grpcService *kgateway.OpenTelemetryAccessLogService, grpcBackends map[string]*ir.BackendObjectIR, accessLogId int) (proto.Message, error) {
	var cfg envoy_open_telemetry.OpenTelemetryAccessLogConfig
	if err := copyOTelSettings(&cfg, grpcService, grpcBackends, accessLogId); err != nil {
		return nil, fmt.Errorf("error converting otel access log config: %w", err)
	}
	return &cfg, nil
}

// addAccessLogFilter adds filtering logic to an access log configuration
func addAccessLogFilter(accessLogCfg *envoyaccesslogv3.AccessLog, filter *kgateway.AccessLogFilter) error {
	var (
		filters []*envoyaccesslogv3.AccessLogFilter
		err     error
	)

	switch {
	case filter.OrFilter != nil:
		filters, err = translateFilters(filter.OrFilter)
		if err != nil {
			return err
		}
		if accessLogCfg.Filter == nil {
			accessLogCfg.Filter = &envoyaccesslogv3.AccessLogFilter{}
		}
		accessLogCfg.Filter.FilterSpecifier = &envoyaccesslogv3.AccessLogFilter_OrFilter{
			OrFilter: &envoyaccesslogv3.OrFilter{Filters: filters},
		}
	case filter.AndFilter != nil:
		filters, err = translateFilters(filter.AndFilter)
		if err != nil {
			return err
		}
		if accessLogCfg.Filter == nil {
			accessLogCfg.Filter = &envoyaccesslogv3.AccessLogFilter{}
		}
		accessLogCfg.Filter.FilterSpecifier = &envoyaccesslogv3.AccessLogFilter_AndFilter{
			AndFilter: &envoyaccesslogv3.AndFilter{Filters: filters},
		}
	case filter.FilterType != nil:
		accessLogCfg.Filter, err = translateFilter(filter.FilterType)
		if err != nil {
			return err
		}
	}

	return nil
}

// translateFilters translates a slice of filter types
func translateFilters(filters []kgateway.FilterType) ([]*envoyaccesslogv3.AccessLogFilter, error) {
	result := make([]*envoyaccesslogv3.AccessLogFilter, 0, len(filters))
	for _, filter := range filters {
		cfg, err := translateFilter(&filter)
		if err != nil {
			return nil, err
		}
		result = append(result, cfg)
	}
	return result, nil
}

func translateFilter(filter *kgateway.FilterType) (*envoyaccesslogv3.AccessLogFilter, error) {
	var alCfg *envoyaccesslogv3.AccessLogFilter
	switch {
	case filter.StatusCodeFilter != nil:
		op, err := toEnvoyComparisonOpType(filter.StatusCodeFilter.Op)
		if err != nil {
			return nil, err
		}

		alCfg = &envoyaccesslogv3.AccessLogFilter{
			FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_StatusCodeFilter{
				StatusCodeFilter: &envoyaccesslogv3.StatusCodeFilter{
					Comparison: &envoyaccesslogv3.ComparisonFilter{
						Op: op,
						Value: &envoycorev3.RuntimeUInt32{
							DefaultValue: uint32(filter.StatusCodeFilter.Value), // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
						},
					},
				},
			},
		}

	case filter.DurationFilter != nil:
		op, err := toEnvoyComparisonOpType(filter.DurationFilter.Op)
		if err != nil {
			return nil, err
		}

		alCfg = &envoyaccesslogv3.AccessLogFilter{
			FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_DurationFilter{
				DurationFilter: &envoyaccesslogv3.DurationFilter{
					Comparison: &envoyaccesslogv3.ComparisonFilter{
						Op: op,
						Value: &envoycorev3.RuntimeUInt32{
							DefaultValue: uint32(filter.DurationFilter.Value), // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
						},
					},
				},
			},
		}

	case filter.NotHealthCheckFilter != nil:
		alCfg = &envoyaccesslogv3.AccessLogFilter{
			FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_NotHealthCheckFilter{
				NotHealthCheckFilter: &envoyaccesslogv3.NotHealthCheckFilter{},
			},
		}

	case filter.TraceableFilter != nil:
		alCfg = &envoyaccesslogv3.AccessLogFilter{
			FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_TraceableFilter{
				TraceableFilter: &envoyaccesslogv3.TraceableFilter{},
			},
		}

	case filter.HeaderFilter != nil:
		alCfg = &envoyaccesslogv3.AccessLogFilter{
			FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_HeaderFilter{
				HeaderFilter: &envoyaccesslogv3.HeaderFilter{
					Header: &envoyroutev3.HeaderMatcher{
						Name:                 string(filter.HeaderFilter.Header.Name),
						HeaderMatchSpecifier: createHeaderMatchSpecifier(filter.HeaderFilter.Header),
					},
				},
			},
		}

	case filter.ResponseFlagFilter != nil:
		alCfg = &envoyaccesslogv3.AccessLogFilter{
			FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_ResponseFlagFilter{
				ResponseFlagFilter: &envoyaccesslogv3.ResponseFlagFilter{
					Flags: filter.ResponseFlagFilter.Flags,
				},
			},
		}

	case filter.GrpcStatusFilter != nil:
		statuses := make([]envoyaccesslogv3.GrpcStatusFilter_Status, len(filter.GrpcStatusFilter.Statuses))
		for i, status := range filter.GrpcStatusFilter.Statuses {
			envoyGrpcStatusType, err := toEnvoyGRPCStatusType(status)
			if err != nil {
				return nil, err
			}
			statuses[i] = envoyGrpcStatusType
		}

		alCfg = &envoyaccesslogv3.AccessLogFilter{
			FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_GrpcStatusFilter{
				GrpcStatusFilter: &envoyaccesslogv3.GrpcStatusFilter{
					Statuses: statuses,
					Exclude:  ptr.Deref(filter.GrpcStatusFilter.Exclude, false),
				},
			},
		}

	case filter.CELFilter != nil:
		celExpressionFilter := &cel.ExpressionFilter{
			Expression: filter.CELFilter.Match,
		}
		celCfg, err := utils.MessageToAny(celExpressionFilter)
		if err != nil {
			logger.Error("error converting CEL filter", "error", err)
			return nil, err
		}

		alCfg = &envoyaccesslogv3.AccessLogFilter{
			FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_ExtensionFilter{
				ExtensionFilter: &envoyaccesslogv3.ExtensionFilter{
					Name: kwellknown.CELExtensionFilter,
					ConfigType: &envoyaccesslogv3.ExtensionFilter_TypedConfig{
						TypedConfig: celCfg,
					},
				},
			},
		}

	default:
		return nil, fmt.Errorf("no valid filter type specified")
	}

	return alCfg, nil
}

// Helper function to create header match specifier
func createHeaderMatchSpecifier(header gwv1.HTTPHeaderMatch) *envoyroutev3.HeaderMatcher_StringMatch {
	switch *header.Type {
	case gwv1.HeaderMatchExact:
		return &envoyroutev3.HeaderMatcher_StringMatch{
			StringMatch: &envoymatcher.StringMatcher{
				IgnoreCase: false,
				MatchPattern: &envoymatcher.StringMatcher_Exact{
					Exact: header.Value,
				},
			},
		}
	case gwv1.HeaderMatchRegularExpression:
		return &envoyroutev3.HeaderMatcher_StringMatch{
			StringMatch: &envoymatcher.StringMatcher{
				IgnoreCase: false,
				MatchPattern: &envoymatcher.StringMatcher_SafeRegex{
					SafeRegex: &envoymatcher.RegexMatcher{
						Regex: header.Value,
					},
				},
			},
		}
	default:
		logger.Error("unsupported header match type", "type", *header.Type)
		return nil
	}
}

func convertJsonFormat(jsonFormat *runtime.RawExtension) (*structpb.Struct, error) {
	if jsonFormat == nil {
		return nil, nil
	}

	var formatMap map[string]any
	if err := json.Unmarshal(jsonFormat.Raw, &formatMap); err != nil {
		return nil, fmt.Errorf("invalid access log jsonFormat: %w", err)
	}

	structVal, err := structpb.NewStruct(formatMap)
	if err != nil {
		return nil, fmt.Errorf("invalid access log jsonFormat: %w", err)
	}

	return structVal, nil
}

func generateCommonAccessLogGrpcConfig(grpcService kgateway.CommonAccessLogGrpcService, grpcBackends map[string]*ir.BackendObjectIR, accessLogId int) (*envoygrpc.CommonGrpcAccessLogConfig, error) {
	if grpcService.LogName == "" {
		return nil, errors.New("grpc service log name cannot be empty")
	}

	backend := grpcBackends[getLogId(grpcService.LogName, accessLogId)]
	if backend == nil {
		return nil, errors.New("backend ref not found")
	}

	commonConfig, err := ToEnvoyGrpc(grpcService.CommonGrpcService, backend)
	if err != nil {
		return nil, err
	}

	return &envoygrpc.CommonGrpcAccessLogConfig{
		LogName:             grpcService.LogName,
		GrpcService:         commonConfig,
		TransportApiVersion: envoycorev3.ApiVersion_V3,
	}, nil
}

func copyGrpcSettings(cfg *envoygrpc.HttpGrpcAccessLogConfig, grpcService *kgateway.AccessLogGrpcService, grpcBackends map[string]*ir.BackendObjectIR, accessLogId int) error {
	config, err := generateCommonAccessLogGrpcConfig(grpcService.CommonAccessLogGrpcService, grpcBackends, accessLogId)
	if err != nil {
		return err
	}

	cfg.CommonConfig = config
	cfg.AdditionalRequestHeadersToLog = grpcService.AdditionalRequestHeadersToLog
	cfg.AdditionalResponseHeadersToLog = grpcService.AdditionalResponseHeadersToLog
	cfg.AdditionalResponseTrailersToLog = grpcService.AdditionalResponseTrailersToLog
	return cfg.Validate()
}

func copyOTelSettings(cfg *envoy_open_telemetry.OpenTelemetryAccessLogConfig, otelService *kgateway.OpenTelemetryAccessLogService, grpcBackends map[string]*ir.BackendObjectIR, accessLogId int) error {
	config, err := generateCommonAccessLogGrpcConfig(otelService.GrpcService, grpcBackends, accessLogId)
	if err != nil {
		return err
	}

	cfg.CommonConfig = config
	if otelService.Body != nil {
		cfg.Body = &otelv1.AnyValue{
			Value: &otelv1.AnyValue_StringValue{
				StringValue: *otelService.Body,
			},
		}
	}
	if otelService.ResourceAttributes != nil {
		cfg.ResourceAttributes = ToOTelKeyValueList(otelService.ResourceAttributes)
	}
	if otelService.DisableBuiltinLabels != nil {
		cfg.DisableBuiltinLabels = *otelService.DisableBuiltinLabels
	}
	if otelService.Attributes != nil {
		cfg.Attributes = ToOTelKeyValueList(otelService.Attributes)
	}

	return cfg.Validate()
}

func ToOTelKeyValueList(in *kgateway.KeyAnyValueList) *otelv1.KeyValueList {
	kvList := make([]*otelv1.KeyValue, len(in.Values))
	ret := &otelv1.KeyValueList{
		Values: kvList,
	}
	for i, value := range in.Values {
		ret.GetValues()[i] = &otelv1.KeyValue{
			Key:   value.Key,
			Value: ToOTelAnyValue(&value.Value),
		}
	}
	return ret
}

func ToOTelAnyValue(in *kgateway.AnyValue) *otelv1.AnyValue {
	if in == nil {
		return nil
	}
	if in.StringValue != nil {
		return &otelv1.AnyValue{
			Value: &otelv1.AnyValue_StringValue{
				StringValue: *in.StringValue,
			},
		}
	}
	if in.ArrayValue != nil {
		arrayValue := &otelv1.AnyValue_ArrayValue{
			ArrayValue: &otelv1.ArrayValue{
				Values: make([]*otelv1.AnyValue, len(in.ArrayValue)),
			},
		}
		for i, value := range in.ArrayValue {
			arrayValue.ArrayValue.GetValues()[i] = ToOTelAnyValue(&value)
		}
		return &otelv1.AnyValue{
			Value: arrayValue,
		}
	}
	if in.KvListValue != nil {
		return &otelv1.AnyValue{
			Value: &otelv1.AnyValue_KvlistValue{
				KvlistValue: ToOTelKeyValueList(in.KvListValue),
			},
		}
	}
	return nil
}

func getFormatterExtensions() ([]*envoycorev3.TypedExtensionConfig, error) {
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

	return []*envoycorev3.TypedExtensionConfig{
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

func newAccessLogWithConfig(name string, config proto.Message) *envoyaccesslogv3.AccessLog {
	s := &envoyaccesslogv3.AccessLog{
		Name: name,
	}

	if config != nil {
		s.ConfigType = &envoyaccesslogv3.AccessLog_TypedConfig{
			TypedConfig: utils.MustMessageToAny(config),
		}
	}

	return s
}

// String provides a string representation for the Op enum.
func toEnvoyComparisonOpType(op kgateway.Op) (envoyaccesslogv3.ComparisonFilter_Op, error) {
	switch op {
	case kgateway.EQ:
		return envoyaccesslogv3.ComparisonFilter_EQ, nil
	case kgateway.GE:
		return envoyaccesslogv3.ComparisonFilter_GE, nil
	case kgateway.LE:
		return envoyaccesslogv3.ComparisonFilter_LE, nil
	default:
		return 0, fmt.Errorf("unknown OP (%s)", op)
	}
}

func toEnvoyGRPCStatusType(grpcStatus kgateway.GrpcStatus) (envoyaccesslogv3.GrpcStatusFilter_Status, error) {
	switch grpcStatus {
	case kgateway.OK:
		return envoyaccesslogv3.GrpcStatusFilter_OK, nil
	case kgateway.CANCELED:
		return envoyaccesslogv3.GrpcStatusFilter_CANCELED, nil
	case kgateway.UNKNOWN:
		return envoyaccesslogv3.GrpcStatusFilter_UNKNOWN, nil
	case kgateway.INVALID_ARGUMENT:
		return envoyaccesslogv3.GrpcStatusFilter_INVALID_ARGUMENT, nil
	case kgateway.DEADLINE_EXCEEDED:
		return envoyaccesslogv3.GrpcStatusFilter_DEADLINE_EXCEEDED, nil
	case kgateway.NOT_FOUND:
		return envoyaccesslogv3.GrpcStatusFilter_NOT_FOUND, nil
	case kgateway.ALREADY_EXISTS:
		return envoyaccesslogv3.GrpcStatusFilter_ALREADY_EXISTS, nil
	case kgateway.PERMISSION_DENIED:
		return envoyaccesslogv3.GrpcStatusFilter_PERMISSION_DENIED, nil
	case kgateway.RESOURCE_EXHAUSTED:
		return envoyaccesslogv3.GrpcStatusFilter_RESOURCE_EXHAUSTED, nil
	case kgateway.FAILED_PRECONDITION:
		return envoyaccesslogv3.GrpcStatusFilter_FAILED_PRECONDITION, nil
	case kgateway.ABORTED:
		return envoyaccesslogv3.GrpcStatusFilter_ABORTED, nil
	case kgateway.OUT_OF_RANGE:
		return envoyaccesslogv3.GrpcStatusFilter_OUT_OF_RANGE, nil
	case kgateway.UNIMPLEMENTED:
		return envoyaccesslogv3.GrpcStatusFilter_UNIMPLEMENTED, nil
	case kgateway.INTERNAL:
		return envoyaccesslogv3.GrpcStatusFilter_INTERNAL, nil
	case kgateway.UNAVAILABLE:
		return envoyaccesslogv3.GrpcStatusFilter_UNAVAILABLE, nil
	case kgateway.DATA_LOSS:
		return envoyaccesslogv3.GrpcStatusFilter_DATA_LOSS, nil
	case kgateway.UNAUTHENTICATED:
		return envoyaccesslogv3.GrpcStatusFilter_UNAUTHENTICATED, nil
	default:
		return 0, fmt.Errorf("unknown GRPCStatus (%s)", grpcStatus)
	}
}

func generateAccessLogConfig(pCtx *ir.HcmContext, policies []kgateway.AccessLog, configs []proto.Message) ([]*envoyaccesslogv3.AccessLog, error) {
	accessLogs := make([]*envoyaccesslogv3.AccessLog, len(configs))
	if len(configs) == 0 {
		return accessLogs, nil
	}

	for i, config := range configs {
		var cfg *envoyaccesslogv3.AccessLog
		switch t := config.(type) {
		case *envoyalfile.FileAccessLog:
			cfg = newAccessLogWithConfig(wellknown.FileAccessLog, t)
		case *envoygrpc.HttpGrpcAccessLogConfig:
			cfg = newAccessLogWithConfig(wellknown.HTTPGRPCAccessLog, t)
		case *envoy_open_telemetry.OpenTelemetryAccessLogConfig:
			addDefaultResourceAttributes(pCtx, t)
			cfg = newAccessLogWithConfig("envoy.access_loggers.open_telemetry", t)
		}
		// Add filter if specified
		if policies[i].Filter != nil {
			if err := addAccessLogFilter(cfg, policies[i].Filter); err != nil {
				return nil, err
			}
		}
		accessLogs[i] = cfg
	}
	return accessLogs, nil
}

func addDefaultResourceAttributes(pCtx *ir.HcmContext, config *envoy_open_telemetry.OpenTelemetryAccessLogConfig) {
	if config.GetResourceAttributes() == nil {
		config.ResourceAttributes = &otelv1.KeyValueList{
			Values: []*otelv1.KeyValue{{
				Key: serviceNameKey,
				Value: &otelv1.AnyValue{
					Value: &otelv1.AnyValue_StringValue{
						StringValue: GenerateDefaultServiceName(pCtx.Gateway.SourceObject.GetName(), pCtx.Gateway.SourceObject.GetNamespace()),
					},
				}},
			},
		}
		return
	}

	for _, ra := range config.GetResourceAttributes().Values {
		if ra.Key == serviceNameKey {
			return
		}
	}
	config.GetResourceAttributes().Values = append(config.GetResourceAttributes().Values, &otelv1.KeyValue{
		Key: serviceNameKey,
		Value: &otelv1.AnyValue{
			Value: &otelv1.AnyValue_StringValue{
				StringValue: GenerateDefaultServiceName(pCtx.Gateway.SourceObject.GetName(), pCtx.Gateway.SourceObject.GetNamespace()),
			},
		},
	})
}
