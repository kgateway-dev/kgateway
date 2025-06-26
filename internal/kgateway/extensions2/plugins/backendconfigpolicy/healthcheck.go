package backendconfigpolicy

import (
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func translateHealthCheck(hc *v1alpha1.HealthCheck) *corev3.HealthCheck {
	if hc == nil {
		return nil
	}

	healthCheck := &corev3.HealthCheck{}

	if hc.Timeout != nil {
		healthCheck.Timeout = durationpb.New(hc.Timeout.Duration)
	}
	if hc.Interval != nil {
		healthCheck.Interval = durationpb.New(hc.Interval.Duration)
	}
	if hc.UnhealthyThreshold != nil {
		healthCheck.UnhealthyThreshold = &wrapperspb.UInt32Value{Value: *hc.UnhealthyThreshold}
	}
	if hc.HealthyThreshold != nil {
		healthCheck.HealthyThreshold = &wrapperspb.UInt32Value{Value: *hc.HealthyThreshold}
	}

	if hc.Http != nil {
		httpHealthCheck := &corev3.HealthCheck_HttpHealthCheck{
			Path: hc.Http.Path,
		}
		if hc.Http.Host != nil {
			httpHealthCheck.Host = *hc.Http.Host
		}
		if hc.Http.Method != nil {
			httpHealthCheck.Method = corev3.RequestMethod(corev3.RequestMethod_value[*hc.Http.Method])
		}
		healthCheck.HealthChecker = &corev3.HealthCheck_HttpHealthCheck_{
			HttpHealthCheck: httpHealthCheck,
		}
	} else if hc.Grpc != nil {
		healthCheck.HealthChecker = &corev3.HealthCheck_GrpcHealthCheck_{
			GrpcHealthCheck: &corev3.HealthCheck_GrpcHealthCheck{},
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
