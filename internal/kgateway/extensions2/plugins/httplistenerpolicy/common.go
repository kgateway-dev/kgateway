package httplistenerpolicy

import (
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
)

func ToEnvoyGrpc(in v1alpha1.CommonGrpcService, backend *ir.BackendObjectIR) (*envoycore.GrpcService, error) {
	envoyGrpcService := &envoycore.GrpcService_EnvoyGrpc{
		ClusterName: backend.ClusterName(),
	}
	if in.Authority != nil {
		envoyGrpcService.Authority = *in.Authority
	}
	if in.MaxReceiveMessageLength != nil {
		envoyGrpcService.MaxReceiveMessageLength = &wrapperspb.UInt32Value{
			Value: *in.MaxReceiveMessageLength,
		}
	}
	if in.SkipEnvoyHeaders != nil {
		envoyGrpcService.SkipEnvoyHeaders = *in.SkipEnvoyHeaders
	}
	grpcService := &envoycore.GrpcService{
		TargetSpecifier: &envoycore.GrpcService_EnvoyGrpc_{
			EnvoyGrpc: envoyGrpcService,
		},
	}

	if in.Timeout != nil {
		grpcService.Timeout = utils.DurationToProto(in.Timeout.Duration)
	}
	if in.InitialMetadata != nil {
		grpcService.InitialMetadata = make([]*envoycore.HeaderValue, len(in.InitialMetadata))
		for i, metadata := range in.InitialMetadata {
			grpcService.GetInitialMetadata()[i] = &envoycore.HeaderValue{
				Key:   metadata.Key,
				Value: metadata.Value,
			}
		}
	}
	if in.RetryPolicy != nil {
		retryPolicy := &envoycore.RetryPolicy{}
		if in.RetryPolicy.NumRetries != nil {
			retryPolicy.NumRetries = &wrapperspb.UInt32Value{
				Value: *in.RetryPolicy.NumRetries,
			}
		}
		if in.RetryPolicy.RetryBackOff != nil {
			retryPolicy.RetryBackOff = &envoycore.BackoffStrategy{
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
