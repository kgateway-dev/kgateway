package trafficpolicy

import (
	"errors"
	"fmt"

	routeconfv3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

const (
	rateLimitFilterName = "envoy.filters.http.ratelimit"
	rateLimitStatPrefix = "http_rate_limit"
)

// RateLimitIR represents the intermediate representation of a rate limit policy
type RateLimitIR struct {
	provider         *TrafficPolicyGatewayExtensionIR
	rateLimitActions []*routeconfv3.RateLimit
}

// createRateLimitActions translates the API descriptors to Envoy route config rate limit actions
func createRateLimitActions(descriptors []v1alpha1.RateLimitDescriptor) ([]*routeconfv3.RateLimit_Action, error) {
	if len(descriptors) == 0 {
		return nil, errors.New("at least one descriptor is required for global rate limiting")
	}

	var result []*routeconfv3.RateLimit_Action

	// Process each descriptor
	for _, descriptor := range descriptors {
		// Each descriptor becomes a separate RateLimit in Envoy with its own set of actions
		// Create actions for each entry in the descriptor
		var actions []*routeconfv3.RateLimit_Action

		for _, entry := range descriptor.Entries {
			action := &routeconfv3.RateLimit_Action{}

			// Set the action specifier based on entry type
			switch entry.Type {
			case v1alpha1.RateLimitDescriptorEntryTypeGeneric:
				if entry.Generic == nil {
					return nil, fmt.Errorf("generic entry requires Generic field to be set")
				}
				action.ActionSpecifier = &routeconfv3.RateLimit_Action_GenericKey_{
					GenericKey: &routeconfv3.RateLimit_Action_GenericKey{
						DescriptorKey:   entry.Generic.Key,
						DescriptorValue: entry.Generic.Value,
					},
				}
			case v1alpha1.RateLimitDescriptorEntryTypeHeader:
				if entry.Header == "" {
					return nil, fmt.Errorf("header entry requires Header field to be set")
				}
				action.ActionSpecifier = &routeconfv3.RateLimit_Action_RequestHeaders_{
					RequestHeaders: &routeconfv3.RateLimit_Action_RequestHeaders{
						HeaderName:    entry.Header,
						DescriptorKey: entry.Header, // Use header name as key
					},
				}
			case v1alpha1.RateLimitDescriptorEntryTypeRemoteAddress:
				action.ActionSpecifier = &routeconfv3.RateLimit_Action_RemoteAddress_{
					RemoteAddress: &routeconfv3.RateLimit_Action_RemoteAddress{},
				}
			case v1alpha1.RateLimitDescriptorEntryTypePath:
				action.ActionSpecifier = &routeconfv3.RateLimit_Action_RequestHeaders_{
					RequestHeaders: &routeconfv3.RateLimit_Action_RequestHeaders{
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
			rateLimit := &routeconfv3.RateLimit{
				Actions: actions,
			}

			// The final result is a slice of complete RateLimit objects
			result = append(result, rateLimit.GetActions()...)
		}
	}

	return result, nil
}
