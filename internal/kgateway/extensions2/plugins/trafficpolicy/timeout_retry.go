package trafficpolicy

import (
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
)

var (
	_ PolicySubIR = &retryIR{}
	_ PolicySubIR = &timeoutsIR{}
)

type retryIR struct {
	policy *envoyroutev3.RetryPolicy
}

func (a *retryIR) Equals(other PolicySubIR) bool {
	b, ok := other.(*retryIR)
	if !ok {
		return false
	}
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return proto.Equal(a.policy, b.policy)
}

func (a *retryIR) Validate() error {
	if a == nil || a.policy == nil {
		return nil
	}
	return a.policy.Validate()
}

type timeoutsIR struct {
	routeTimeout           *durationpb.Duration
	routeStreamIdleTimeout *durationpb.Duration
}

func (a *timeoutsIR) Equals(other PolicySubIR) bool {
	b, ok := other.(*timeoutsIR)
	if !ok {
		return false
	}
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return proto.Equal(a.routeTimeout, b.routeTimeout) &&
		proto.Equal(a.routeStreamIdleTimeout, b.routeStreamIdleTimeout)
}

func (a *timeoutsIR) Validate() error {
	return nil
}

func constructTimeoutRetry(
	spec v1alpha1.TrafficPolicySpec,
	out *trafficPolicySpecIr,
) {
	if spec.Timeouts != nil {
		out.timeouts = &timeoutsIR{}
		if spec.Timeouts.Request != nil {
			out.timeouts.routeTimeout = durationpb.New(spec.Timeouts.Request.Duration)
		}
		if spec.Timeouts.StreamIdle != nil {
			out.timeouts.routeStreamIdleTimeout = durationpb.New(spec.Timeouts.StreamIdle.Duration)
		}
	}

	if spec.Retry != nil {
		out.retry = &retryIR{
			policy: policy.BuildRetryPolicy(spec.Retry),
		}
	}
}
