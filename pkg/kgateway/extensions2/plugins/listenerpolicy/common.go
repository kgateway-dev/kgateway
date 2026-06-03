package listenerpolicy

import (
	"fmt"

	envoyaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	cel "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/filters/cel/v3"
	envoytypev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	kwellknown "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	pluginsdkutils "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/utils"
)

func ToEnvoyGrpc(in kgateway.CommonGrpcService, backend *ir.BackendObjectIR) (*envoycorev3.GrpcService, error) {
	envoyGrpcService := &envoycorev3.GrpcService_EnvoyGrpc{
		ClusterName: backend.ClusterName(),
	}
	if in.Authority != nil {
		envoyGrpcService.Authority = *in.Authority
	}
	if in.MaxReceiveMessageLength != nil {
		envoyGrpcService.MaxReceiveMessageLength = &wrapperspb.UInt32Value{
			Value: uint32(*in.MaxReceiveMessageLength), // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
		}
	}
	if in.SkipEnvoyHeaders != nil {
		envoyGrpcService.SkipEnvoyHeaders = *in.SkipEnvoyHeaders
	}
	grpcService := &envoycorev3.GrpcService{
		TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
			EnvoyGrpc: envoyGrpcService,
		},
	}

	if in.Timeout != nil {
		grpcService.Timeout = utils.DurationToProto(in.Timeout.Duration)
	}
	if in.InitialMetadata != nil {
		grpcService.InitialMetadata = make([]*envoycorev3.HeaderValue, len(in.InitialMetadata))
		for i, metadata := range in.InitialMetadata {
			grpcService.GetInitialMetadata()[i] = &envoycorev3.HeaderValue{
				Key:   metadata.Key,
				Value: ptr.Deref(metadata.Value, ""),
			}
		}
	}
	if in.RetryPolicy != nil {
		retryPolicy := &envoycorev3.RetryPolicy{}
		if in.RetryPolicy.NumRetries != nil {
			retryPolicy.NumRetries = &wrapperspb.UInt32Value{
				Value: uint32(*in.RetryPolicy.NumRetries), // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
			}
		}
		if in.RetryPolicy.RetryBackOff != nil {
			retryPolicy.RetryBackOff = &envoycorev3.BackoffStrategy{
				BaseInterval: utils.DurationToProto(in.RetryPolicy.RetryBackOff.BaseInterval.Duration),
			}
			if in.RetryPolicy.RetryBackOff.MaxInterval != nil {
				if in.RetryPolicy.RetryBackOff.MaxInterval.Duration.Nanoseconds() < in.RetryPolicy.RetryBackOff.BaseInterval.Duration.Nanoseconds() {
					logger.Error("retryPolicy.RetryBackOff.MaxInterval is lesser than RetryPolicy.RetryBackOff.MaxInterval. Ignoring MaxInterval", "max_interval", in.RetryPolicy.RetryBackOff.MaxInterval.Duration.Seconds(), "base_interval", in.RetryPolicy.RetryBackOff.BaseInterval.Duration.Seconds())
				} else {
					retryPolicy.GetRetryBackOff().MaxInterval = utils.DurationToProto(in.RetryPolicy.RetryBackOff.MaxInterval.Duration)
				}
			}
		}
		grpcService.RetryPolicy = retryPolicy
	}
	return grpcService, nil
}

// convertAccessLogFilter translates filtering logic to Envoy filter
func convertAccessLogFilter(filter *kgateway.AccessLogFilter) (*envoyaccesslogv3.AccessLogFilter, error) {
	var (
		filters []*envoyaccesslogv3.AccessLogFilter
		err     error
	)

	switch {
	case filter.OrFilter != nil:
		filters, err = translateFilters(filter.OrFilter)
		if err != nil {
			return nil, err
		}
		return &envoyaccesslogv3.AccessLogFilter{
			FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_OrFilter{
				OrFilter: &envoyaccesslogv3.OrFilter{Filters: filters},
			},
		}, nil
	case filter.AndFilter != nil:
		filters, err = translateFilters(filter.AndFilter)
		if err != nil {
			return nil, err
		}
		return &envoyaccesslogv3.AccessLogFilter{
			FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_AndFilter{
				AndFilter: &envoyaccesslogv3.AndFilter{Filters: filters},
			},
		}, nil
	case filter.FilterType != nil:
		return translateFilter(filter.FilterType)
	}

	return nil, nil
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
							DefaultValue: filter.StatusCodeFilter.Value,
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
							DefaultValue: filter.DurationFilter.Value,
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
		matcher, err := pluginsdkutils.ToEnvoyHeaderMatcher(filter.HeaderFilter.Header)
		if err != nil {
			return nil, err
		}
		alCfg = &envoyaccesslogv3.AccessLogFilter{
			FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_HeaderFilter{
				HeaderFilter: &envoyaccesslogv3.HeaderFilter{
					Header: matcher,
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

	case filter.RuntimeFilter != nil:
		rf := &envoyaccesslogv3.RuntimeFilter{
			RuntimeKey: filter.RuntimeFilter.RuntimeKey,
		}
		if filter.RuntimeFilter.PercentSampled != nil {
			rf.PercentSampled = &envoytypev3.FractionalPercent{
				Numerator: uint32(filter.RuntimeFilter.PercentSampled.Numerator), // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
			}
			if filter.RuntimeFilter.PercentSampled.Denominator != nil {
				denominator, err := toEnvoyDenominatorType(*filter.RuntimeFilter.PercentSampled.Denominator)
				if err != nil {
					return nil, err
				}
				rf.PercentSampled.Denominator = denominator
			}
		}
		if filter.RuntimeFilter.UseIndependentRandomness != nil {
			rf.UseIndependentRandomness = *filter.RuntimeFilter.UseIndependentRandomness
		}
		alCfg = &envoyaccesslogv3.AccessLogFilter{
			FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_RuntimeFilter{
				RuntimeFilter: rf,
			},
		}

	default:
		return nil, fmt.Errorf("no valid filter type specified")
	}

	return alCfg, nil
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

func toEnvoyDenominatorType(denominator kgateway.DenominatorType) (envoytypev3.FractionalPercent_DenominatorType, error) {
	switch denominator {
	case kgateway.HUNDRED:
		return envoytypev3.FractionalPercent_HUNDRED, nil
	case kgateway.TEN_THOUSAND:
		return envoytypev3.FractionalPercent_TEN_THOUSAND, nil
	case kgateway.MILLION:
		return envoytypev3.FractionalPercent_MILLION, nil
	default:
		return 0, fmt.Errorf("unknown DenominatorType (%s)", denominator)
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
