package trafficpolicy

import (
	"errors"
	"fmt"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	ratev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmputils"
)

const (
	rateLimitStatPrefix = "http_rate_limit"
)

// globalRateLimitIR represents the intermediate representation for a global rate limit policy.
type globalRateLimitIR struct {
	provider         *TrafficPolicyGatewayExtensionIR
	rateLimitActions []*envoyroutev3.RateLimit
}

var _ PolicySubIR = &globalRateLimitIR{}

// Equals checks if two globalRateLimitIR instances are equal.
func (r *globalRateLimitIR) Equals(other PolicySubIR) bool {
	otherGlobalRateLimit, ok := other.(*globalRateLimitIR)
	if !ok {
		return false
	}
	if r == nil && otherGlobalRateLimit == nil {
		return true
	}
	if r == nil || otherGlobalRateLimit == nil {
		return false
	}

	if len(r.rateLimitActions) != len(otherGlobalRateLimit.rateLimitActions) {
		return false
	}
	for i, action := range r.rateLimitActions {
		if !proto.Equal(action, otherGlobalRateLimit.rateLimitActions[i]) {
			return false
		}
	}
	if !cmputils.CompareWithNils(r.provider, otherGlobalRateLimit.provider, func(a, b *TrafficPolicyGatewayExtensionIR) bool {
		return a.Equals(*b)
	}) {
		return false
	}

	return true
}

func (r *globalRateLimitIR) Validate() error {
	if r == nil {
		return nil
	}
	for _, rateLimit := range r.rateLimitActions {
		if rateLimit != nil {
			if err := rateLimit.ValidateAll(); err != nil {
				return err
			}
		}
	}
	if r.provider != nil {
		if err := r.provider.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// constructGlobalRateLimit constructs the global rate limit policy IR from the policy specification.
func constructGlobalRateLimit(
	krtctx krt.HandlerContext,
	in *v1alpha1.TrafficPolicy,
	fetchGatewayExtension FetchGatewayExtensionFunc,
	out *trafficPolicySpecIr,
) error {
	if in.Spec.RateLimit == nil || in.Spec.RateLimit.Global == nil {
		return nil
	}

	globalPolicy := in.Spec.RateLimit.Global
	// Create rate limit actions for the route or vhost
	actions, err := createRateLimitActions(globalPolicy.Descriptors)
	if err != nil {
		return fmt.Errorf("failed to create rate limit actions: %w", err)
	}
	gwExtIR, err := fetchGatewayExtension(krtctx, globalPolicy.ExtensionRef, in.GetNamespace())
	if err != nil {
		return fmt.Errorf("ratelimit: %w", err)
	}
	if gwExtIR.ExtType != v1alpha1.GatewayExtensionTypeRateLimit || gwExtIR.RateLimit == nil {
		return pluginutils.ErrInvalidExtensionType(v1alpha1.GatewayExtensionTypeExtAuth, gwExtIR.ExtType)
	}
	// Create route rate limits and store in the RateLimitIR struct
	out.globalRateLimit = &globalRateLimitIR{
		provider: gwExtIR,
		rateLimitActions: []*envoyroutev3.RateLimit{
			{
				Actions: actions,
			},
		},
	}
	return nil
}

// createRateLimitActions translates the API descriptors to Envoy route config rate limit actions
func createRateLimitActions(descriptors []v1alpha1.RateLimitDescriptor) ([]*envoyroutev3.RateLimit_Action, error) {
	if len(descriptors) == 0 {
		return nil, errors.New("at least one descriptor is required for global rate limiting")
	}

	var result []*envoyroutev3.RateLimit_Action

	// Process each descriptor
	for _, descriptor := range descriptors {
		// Each descriptor becomes a separate RateLimit in Envoy with its own set of actions
		// Create actions for each entry in the descriptor
		var actions []*envoyroutev3.RateLimit_Action

		for _, entry := range descriptor.Entries {
			action := &envoyroutev3.RateLimit_Action{}

			// Set the action specifier based on entry type
			switch entry.Type {
			case v1alpha1.RateLimitDescriptorEntryTypeGeneric:
				if entry.Generic == nil {
					return nil, fmt.Errorf("generic entry requires Generic field to be set")
				}
				action.ActionSpecifier = &envoyroutev3.RateLimit_Action_GenericKey_{
					GenericKey: &envoyroutev3.RateLimit_Action_GenericKey{
						DescriptorKey:   entry.Generic.Key,
						DescriptorValue: entry.Generic.Value,
					},
				}
			case v1alpha1.RateLimitDescriptorEntryTypeHeader:
				if entry.Header == nil {
					return nil, fmt.Errorf("header entry requires Header field to be set")
				}
				action.ActionSpecifier = &envoyroutev3.RateLimit_Action_RequestHeaders_{
					RequestHeaders: &envoyroutev3.RateLimit_Action_RequestHeaders{
						HeaderName:    *entry.Header,
						DescriptorKey: *entry.Header, // Use header name as key
					},
				}
			case v1alpha1.RateLimitDescriptorEntryTypeRemoteAddress:
				action.ActionSpecifier = &envoyroutev3.RateLimit_Action_RemoteAddress_{
					RemoteAddress: &envoyroutev3.RateLimit_Action_RemoteAddress{},
				}
			case v1alpha1.RateLimitDescriptorEntryTypePath:
				action.ActionSpecifier = &envoyroutev3.RateLimit_Action_RequestHeaders_{
					RequestHeaders: &envoyroutev3.RateLimit_Action_RequestHeaders{
						HeaderName:    ":path",
						DescriptorKey: "path",
					},
				}
			default:
				return nil, fmt.Errorf("unsupported entry type: %s", entry.Type)
			}

			actions = append(actions, action)
		}

		// If we have actions for this descriptor, add it
		if len(actions) > 0 {
			// In Envoy, a single RateLimit includes multiple Actions that together form a descriptor
			rateLimit := &envoyroutev3.RateLimit{
				Actions: actions,
			}

			// The final result is a slice of complete RateLimit objects
			result = append(result, rateLimit.GetActions()...)
		}
	}

	return result, nil
}

func getRateLimitFilterName(name string) string {
	if name == "" {
		return rateLimitFilterNamePrefix
	}
	return fmt.Sprintf("%s/%s", rateLimitFilterNamePrefix, name)
}

// handleGlobalRateLimit adds rate limit configurations to routes
func (p *trafficPolicyPluginGwPass) handleGlobalRateLimit(fcn string, typedFilterConfig *ir.TypedFilterConfigMap, globalRateLimit *globalRateLimitIR) {
	if globalRateLimit == nil {
		return
	}
	if globalRateLimit.rateLimitActions == nil {
		return
	}

	providerName := globalRateLimit.provider.ResourceName()

	// Initialize the map if it doesn't exist yet
	p.rateLimitPerProvider.Add(fcn, providerName, globalRateLimit.provider)

	// Configure rate limit per route - enabling it for this specific route
	rateLimitPerRoute := &ratev3.RateLimitPerRoute{
		RateLimits: globalRateLimit.rateLimitActions,
	}
	typedFilterConfig.AddTypedConfig(getRateLimitFilterName(providerName), rateLimitPerRoute)
}
