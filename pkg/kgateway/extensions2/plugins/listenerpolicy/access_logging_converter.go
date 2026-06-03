package listenerpolicy

import (
	"errors"
	"fmt"

	envoyaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyalfile "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	envoygrpc "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3"
	envoy_open_telemetry "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/open_telemetry/v3"
	envoy_metadata_formatter "github.com/envoyproxy/go-control-plane/envoy/extensions/formatter/metadata/v3"
	envoy_req_without_query "github.com/envoyproxy/go-control-plane/envoy/extensions/formatter/req_without_query/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	otelv1 "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	kwellknown "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/version"
)

var ErrUnresolvedBackendRef = errors.New("unresolved backend reference")

const (
	// resource attribute keys per OTel semantic conventions
	// https://opentelemetry.io/docs/specs/semconv/resource/k8s/

	// Note: attributes such as k8s.pod.name, k8s.pod.uid, etc. cannot be set for access
	// logs because Envoy's OTel access log does not support OTEL_RESOURCE_ATTRIBUTES
	serviceNameKey      = "service.name"
	serviceNamespaceKey = "service.namespace"
	serviceVersionKey   = "service.version"

	k8sNamespaceNameKey = "k8s.namespace.name"
	k8sContainerNameKey = "k8s.container.name"
)

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
		jsonStruct, err := utils.JSONToProtoStruct(fileSink.JsonFormat.Raw)
		if err != nil {
			return nil, fmt.Errorf("invalid access log jsonFormat: %w", err)
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

func generateCommonAccessLogGrpcConfig(grpcService kgateway.CommonAccessLogGrpcService, grpcBackends map[string]*ir.BackendObjectIR, accessLogId int) (*envoygrpc.CommonGrpcAccessLogConfig, error) {
	if grpcService.LogName == "" {
		return nil, errors.New("grpc service log name cannot be empty")
	}

	grpcServiceConfig, err := generateGrpcServiceConfig(grpcService, grpcBackends, accessLogId)
	if err != nil {
		return nil, err
	}

	return &envoygrpc.CommonGrpcAccessLogConfig{
		LogName:             grpcService.LogName,
		GrpcService:         grpcServiceConfig,
		TransportApiVersion: envoycorev3.ApiVersion_V3,
	}, nil
}

func generateGrpcServiceConfig(grpcService kgateway.CommonAccessLogGrpcService, grpcBackends map[string]*ir.BackendObjectIR, accessLogId int) (*envoycorev3.GrpcService, error) {
	backend := grpcBackends[getLogId(grpcService.LogName, accessLogId)]
	if backend == nil {
		return nil, errors.New("backend ref not found")
	}

	return ToEnvoyGrpc(grpcService.CommonGrpcService, backend)
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
	config, err := generateGrpcServiceConfig(otelService.GrpcService, grpcBackends, accessLogId)
	if err != nil {
		return err
	}

	cfg.LogName = otelService.GrpcService.LogName
	cfg.GrpcService = config
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
			filter, err := convertAccessLogFilter(policies[i].Filter)
			if err != nil {
				return nil, err
			}
			cfg.Filter = filter
		}
		accessLogs[i] = cfg
	}
	return accessLogs, nil
}

func addDefaultResourceAttributes(pCtx *ir.HcmContext, config *envoy_open_telemetry.OpenTelemetryAccessLogConfig) {
	gatewayName := pCtx.Gateway.SourceObject.GetName()
	gatewayNamespace := pCtx.Gateway.SourceObject.GetNamespace()

	// Set default resource attributes if not already present
	addResourceAttributeIfMissing(config, serviceNameKey, GenerateDefaultServiceName(gatewayName, gatewayNamespace))
	addResourceAttributeIfMissing(config, serviceNamespaceKey, gatewayNamespace)

	if version.Version != "" {
		addResourceAttributeIfMissing(config, serviceVersionKey, version.Version)
	}

	addResourceAttributeIfMissing(config, k8sNamespaceNameKey, gatewayNamespace)
	addResourceAttributeIfMissing(config, k8sContainerNameKey, kwellknown.KgatewayContainerName)
}

// addResourceAttributeIfMissing adds a string resource attribute to the config
// only if no attribute with the given key already exists.
func addResourceAttributeIfMissing(config *envoy_open_telemetry.OpenTelemetryAccessLogConfig, key, value string) {
	if config.GetResourceAttributes() != nil {
		for _, ra := range config.GetResourceAttributes().Values {
			if ra.Key == key {
				return
			}
		}
	}
	if config.ResourceAttributes == nil {
		config.ResourceAttributes = &otelv1.KeyValueList{}
	}
	config.ResourceAttributes.Values = append(config.ResourceAttributes.Values, &otelv1.KeyValue{
		Key: key,
		Value: &otelv1.AnyValue{
			Value: &otelv1.AnyValue_StringValue{
				StringValue: value,
			},
		},
	})
}
