package trafficpolicy

import (
	"strings"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
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

// applyTimeoutDefaults sets timeout values on routes that don't already have them.
// This is used for gateway/listener-level policies where timeouts act as defaults
// that can be overridden by route-level policies.
func applyTimeoutDefaults(routes []*envoyroutev3.Route, timeouts *timeoutsIR) {
	if timeouts == nil {
		return
	}
	for _, route := range routes {
		action := route.GetRoute()
		if action == nil {
			continue
		}
		// GRPCRoute timeouts are not defined by the Gateway API spec, so
		// gateway-level timeout defaults must not apply to them.
		if isGRPCRoute(route) {
			continue
		}
		if timeouts.routeTimeout != nil && action.GetTimeout() == nil {
			action.Timeout = timeouts.routeTimeout
		}
		if timeouts.routeStreamIdleTimeout != nil && action.GetIdleTimeout() == nil {
			action.IdleTimeout = timeouts.routeStreamIdleTimeout
		}
	}
}

// isGRPCRoute reports whether the given Envoy route originated from a GRPCRoute.
// UniqueRouteName() in query/route.go embeds the lowercase route kind in the
// route name (e.g. "listener~80~...-route-0-grpcroute-<name>-...").
func isGRPCRoute(route *envoyroutev3.Route) bool {
	return strings.Contains(route.GetName(), "-grpcroute-")
}

func constructTimeoutRetry(
	spec kgateway.TrafficPolicySpec,
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
