package httplistenerpolicy

import (
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
)

func ToEnvoyGrpc(in v1alpha1.CommonGrpcService, backend *ir.BackendObjectIR) (*envoycorev3.GrpcService, error) {
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
