package translator

import (
	"fmt"
	"time"

	"github.com/agentgateway/agentgateway/go/api"
	"google.golang.org/protobuf/types/known/durationpb"
	"istio.io/istio/pkg/log"
	"istio.io/istio/pkg/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

// ApplyTimeouts applies timeouts to an agw route
func ApplyTimeouts(rule *gwv1.HTTPRouteRule, route *api.Route) error {
	if rule == nil || rule.Timeouts == nil {
		return nil
	}
	if route.TrafficPolicies == nil {
		route.TrafficPolicies = []*api.TrafficPolicySpec{}
	}
	var reqDur, beDur *durationpb.Duration

	if rule.Timeouts.Request != nil {
		d, err := time.ParseDuration(string(*rule.Timeouts.Request))
		if err != nil {
			return fmt.Errorf("failed to parse request timeout: %w", err)
		}
		if d != 0 {
			// "Setting a timeout to the zero duration (e.g. "0s") SHOULD disable the timeout"
			// However, agentgateway already defaults to no timeout, so only set for non-zero
			reqDur = durationpb.New(d)
		}
	}
	if rule.Timeouts.BackendRequest != nil {
		d, err := time.ParseDuration(string(*rule.Timeouts.BackendRequest))
		if err != nil {
			return fmt.Errorf("failed to parse backend request timeout: %w", err)
		}
		if d != 0 {
			// "Setting a timeout to the zero duration (e.g. "0s") SHOULD disable the timeout"
			// However, agentgateway already defaults to no timeout, so only set for non-zero
			beDur = durationpb.New(d)
		}
	}
	if reqDur != nil || beDur != nil {
		route.TrafficPolicies = append(route.TrafficPolicies, &api.TrafficPolicySpec{
			Kind: &api.TrafficPolicySpec_Timeout{
				Timeout: &api.Timeout{
					Request:        reqDur,
					BackendRequest: beDur,
				},
			},
		})
	}
	return nil
}

// ApplyRetries applies retries to an agw route
func ApplyRetries(rule *gwv1.HTTPRouteRule, route *api.Route) error {
	if rule == nil || rule.Retry == nil {
		return nil
	}
	if a := rule.Retry.Attempts; a != nil && *a == 0 {
		return nil
	}
	if route.TrafficPolicies == nil {
		route.TrafficPolicies = []*api.TrafficPolicySpec{}
	}
	tpRetry := &api.Retry{}
	if rule.Retry.Codes != nil {
		for _, c := range rule.Retry.Codes {
			tpRetry.RetryStatusCodes = append(tpRetry.RetryStatusCodes, int32(c)) //nolint:gosec // G115: HTTP status codes are always positive integers (100-599)
		}
	}
	if rule.Retry.Backoff != nil {
		if d, err := time.ParseDuration(string(*rule.Retry.Backoff)); err == nil {
			tpRetry.Backoff = durationpb.New(d)
		}
	}
	if rule.Retry.Attempts != nil {
		tpRetry.Attempts = int32(*rule.Retry.Attempts) //nolint:gosec // G115: kubebuilder validation ensures 0 <= value, safe for int32
	}
	route.TrafficPolicies = append(route.TrafficPolicies, &api.TrafficPolicySpec{
		Kind: &api.TrafficPolicySpec_Retry{
			Retry: tpRetry,
		},
	})
	return nil
}

func GetStatus[I, IS any](spec I) IS {
	switch t := any(spec).(type) {
	case *gwv1.Gateway:
		return any(t.Status).(IS)
	case *gwv1.HTTPRoute:
		return any(t.Status).(IS)
	case *gwv1.GRPCRoute:
		return any(t.Status).(IS)
	case *gwv1alpha2.TCPRoute:
		return any(t.Status).(IS)
	case *gwv1alpha2.TLSRoute:
		return any(t.Status).(IS)
	case *v1alpha1.TrafficPolicy:
		return any(t.Status).(IS)
	case *gwxv1a1.XListenerSet:
		return any(t.Status).(IS)
	case *v1alpha1.AgentgatewayPolicy:
		return any(t.Status).(IS)
	default:
		log.Fatalf("GetStatus unknown type %T", t)
		return ptr.Empty[IS]()
	}
}
