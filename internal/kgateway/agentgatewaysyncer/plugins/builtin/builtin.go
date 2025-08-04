package builtin

import (
	"fmt"
	"time"

	"github.com/agentgateway/agentgateway/go/api"
	"google.golang.org/protobuf/types/known/durationpb"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	reportssdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

func NewBuiltinPlugin() pluginsdk.Plugin {
	return pluginsdk.Plugin{
		ContributesPolicies: map[schema.GroupKind]pluginsdk.PolicyPlugin{
			ir.VirtualBuiltInGK: {
				NewAgentGatewayPass: func(reporter reportssdk.Reporter) ir.AgentGatewayTranslationPass {
					return NewPass()
				},
			},
		},
	}
}

// Pass implements the ir.AgentGatewayTranslationPass interface.
type Pass struct{}

// NewPass creates a new Pass.
func NewPass() *Pass {
	return &Pass{}
}

// ApplyForRoute applies the builtin transformations for the given route.
func (p *Pass) ApplyForRoute(pctx *ir.AgentGatewayRouteContext, route *api.Route) error {
	var errs []error
	err := applyTimeouts(pctx.Rule, route)
	if err != nil {
		errs = append(errs, err)
	}
	err = applyRetries(pctx.Rule, route)
	if err != nil {
		errs = append(errs, err)
	}
	return utilerrors.NewAggregate(errs)
}

func applyTimeouts(rule *gwv1.HTTPRouteRule, route *api.Route) error {
	if rule.Timeouts != nil {
		if route.TrafficPolicy == nil {
			route.TrafficPolicy = &api.TrafficPolicy{}
		}
		if rule.Timeouts.Request != nil {
			if parsed, err := time.ParseDuration(string(*rule.Timeouts.Request)); err == nil {
				route.TrafficPolicy.RequestTimeout = durationpb.New(parsed)
			} else {
				return fmt.Errorf("failed to parse request timeout: %v", err)
			}
		}
		if rule.Timeouts.BackendRequest != nil {
			if parsed, err := time.ParseDuration(string(*rule.Timeouts.BackendRequest)); err == nil {
				route.TrafficPolicy.BackendRequestTimeout = durationpb.New(parsed)
			} else {
				return fmt.Errorf("failed to parse backend request timeout: %v", err)
			}
		}
	}
	return nil
}

func applyRetries(rule *gwv1.HTTPRouteRule, route *api.Route) error {
	if rule.Retry != nil {
		if route.TrafficPolicy == nil {
			route.TrafficPolicy = &api.TrafficPolicy{}
		}
		route.TrafficPolicy.Retry = &api.Retry{}
		if rule.Retry.Codes != nil {
			var codes []int32
			for _, c := range rule.Retry.Codes {
				codes = append(codes, int32(c))
			}
			route.TrafficPolicy.Retry.RetryStatusCodes = codes
		}
		if rule.Retry.Backoff != nil {
			var ttl *durationpb.Duration
			if parsed, err := time.ParseDuration(string(*rule.Retry.Backoff)); err == nil {
				ttl = durationpb.New(parsed)
			}
			route.TrafficPolicy.Retry.Backoff = ttl
		}
		if rule.Retry.Attempts != nil {
			route.TrafficPolicy.Retry.Attempts = int32(*rule.Retry.Attempts)
		}
	}
	return nil
}
