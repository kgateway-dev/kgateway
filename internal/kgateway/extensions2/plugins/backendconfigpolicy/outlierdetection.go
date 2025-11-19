package backendconfigpolicy

import (
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func translateOutlierDetection(od *v1alpha1.OutlierDetection) *envoyclusterv3.OutlierDetection {
	if od == nil {
		return nil
	}

	outlierDetection := &envoyclusterv3.OutlierDetection{
		Consecutive_5Xx:    &wrapperspb.UInt32Value{Value: uint32(od.Consecutive5xx)}, // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
		Interval:           durationpb.New(od.Interval.Duration),
		BaseEjectionTime:   durationpb.New(od.BaseEjectionTime.Duration),
		MaxEjectionPercent: &wrapperspb.UInt32Value{Value: uint32(od.MaxEjectionPercent)}, // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}

	return outlierDetection
}
