package backendconfigpolicy

import (
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func translateCircuitBreakers(cb *kgateway.CircuitBreakers) *envoyclusterv3.CircuitBreakers {
	if cb == nil {
		return nil
	}

	threshold := &envoyclusterv3.CircuitBreakers_Thresholds{}

	if cb.MaxConnections != nil {
		threshold.MaxConnections = wrapperspb.UInt32(uint32(*cb.MaxConnections)) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	if cb.MaxPendingRequests != nil {
		threshold.MaxPendingRequests = wrapperspb.UInt32(uint32(*cb.MaxPendingRequests)) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	if cb.MaxRequests != nil {
		threshold.MaxRequests = wrapperspb.UInt32(uint32(*cb.MaxRequests)) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	if cb.MaxRetries != nil {
		threshold.MaxRetries = wrapperspb.UInt32(uint32(*cb.MaxRetries)) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}

	return &envoyclusterv3.CircuitBreakers{
		Thresholds: []*envoyclusterv3.CircuitBreakers_Thresholds{threshold},
	}
}
