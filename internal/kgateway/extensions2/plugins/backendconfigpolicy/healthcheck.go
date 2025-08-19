package backendconfigpolicy

import (
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func translateHealthCheck(hc *v1alpha1.HealthCheck) *envoycorev3.HealthCheck {
	if hc == nil {
		return nil
	}

	healthCheck := &envoycorev3.HealthCheck{}

	// These are required fields, so no need to check for nil
	healthCheck.Timeout = durationpb.New(hc.Timeout.Duration)
	healthCheck.Interval = durationpb.New(hc.Interval.Duration)
	healthCheck.UnhealthyThreshold = &wrapperspb.UInt32Value{Value: hc.UnhealthyThreshold}
	healthCheck.HealthyThreshold = &wrapperspb.UInt32Value{Value: hc.HealthyThreshold}

	if hc.Http != nil {
		httpHealthCheck := &envoycorev3.HealthCheck_HttpHealthCheck{
			Path: hc.Http.Path,
		}
		if hc.Http.Host != nil {
			httpHealthCheck.Host = *hc.Http.Host
		}
		if hc.Http.Method != nil {
			httpHealthCheck.Method = envoycorev3.RequestMethod(envoycorev3.RequestMethod_value[*hc.Http.Method])
		}
		healthCheck.HealthChecker = &envoycorev3.HealthCheck_HttpHealthCheck_{
			HttpHealthCheck: httpHealthCheck,
		}
	} else if hc.Grpc != nil {
		healthCheck.HealthChecker = &envoycorev3.HealthCheck_GrpcHealthCheck_{
			GrpcHealthCheck: &envoycorev3.HealthCheck_GrpcHealthCheck{},
		}
		if hc.Grpc.ServiceName != nil {
			healthCheck.GetGrpcHealthCheck().ServiceName = *hc.Grpc.ServiceName
		}
		if hc.Grpc.Authority != nil {
			healthCheck.GetGrpcHealthCheck().Authority = *hc.Grpc.Authority
		}
	}

	return healthCheck
}
