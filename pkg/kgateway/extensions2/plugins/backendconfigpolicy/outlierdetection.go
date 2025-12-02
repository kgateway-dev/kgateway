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

	outlierDetection := &envoyclusterv3.OutlierDetection{}

	if od.Consecutive5xx != nil {
		outlierDetection.Consecutive_5Xx = &wrapperspb.UInt32Value{Value: uint32(*od.Consecutive5xx)} // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	if od.Interval != nil {
		outlierDetection.Interval = durationpb.New(od.Interval.Duration)
	}
	if od.BaseEjectionTime != nil {
		outlierDetection.BaseEjectionTime = durationpb.New(od.BaseEjectionTime.Duration)
	}
	if od.MaxEjectionPercent != nil {
		outlierDetection.MaxEjectionPercent = &wrapperspb.UInt32Value{Value: uint32(*od.MaxEjectionPercent)} // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	return outlierDetection
}
