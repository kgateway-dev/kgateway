package trafficpolicy

import (
	"fmt"
	"sort"

	"google.golang.org/protobuf/types/known/durationpb"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

func hashPolicyForSpec(spec v1alpha1.TrafficPolicySpec, outSpec *trafficPolicySpecIr) error {
	if len(spec.HashPolicies) == 0 {
		return nil
	}

	// We only support attaching hash policies to HTTPRoutes.
	for _, target := range spec.TargetRefs {
		if target.Kind != wellknown.HTTPRouteKind {
			return fmt.Errorf("hash policy is not supported for target kind %s", target.Kind)
		}
	}

	policies := make([]*routev3.RouteAction_HashPolicy, 0, len(spec.HashPolicies))
	for _, hashPolicy := range spec.HashPolicies {
		policy := &routev3.RouteAction_HashPolicy{}
		if hashPolicy.Terminal != nil {
			policy.Terminal = *hashPolicy.Terminal
		}
		switch {
		case hashPolicy.Header != nil:
			policy.PolicySpecifier = &routev3.RouteAction_HashPolicy_Header_{
				Header: &routev3.RouteAction_HashPolicy_Header{
					HeaderName: hashPolicy.Header.Name,
				},
			}
		case hashPolicy.Cookie != nil:
			policy.PolicySpecifier = &routev3.RouteAction_HashPolicy_Cookie_{
				Cookie: &routev3.RouteAction_HashPolicy_Cookie{
					Name: hashPolicy.Cookie.Name,
				},
			}
			if hashPolicy.Cookie.TTL != nil {
				policy.GetCookie().Ttl = durationpb.New(hashPolicy.Cookie.TTL.Duration)
			}
			if hashPolicy.Cookie.Path != nil {
				policy.GetCookie().Path = *hashPolicy.Cookie.Path
			}
			if hashPolicy.Cookie.Attributes != nil {
				// Get all attribute names and sort them for consistent ordering
				names := make([]string, 0, len(hashPolicy.Cookie.Attributes))
				for name := range hashPolicy.Cookie.Attributes {
					names = append(names, name)
				}
				sort.Strings(names)

				attributes := make([]*routev3.RouteAction_HashPolicy_CookieAttribute, 0, len(hashPolicy.Cookie.Attributes))
				for _, name := range names {
					attributes = append(attributes, &routev3.RouteAction_HashPolicy_CookieAttribute{
						Name:  name,
						Value: hashPolicy.Cookie.Attributes[name],
					})
				}
				policy.GetCookie().Attributes = attributes
			}
		case hashPolicy.SourceIP != nil:
			policy.PolicySpecifier = &routev3.RouteAction_HashPolicy_ConnectionProperties_{
				ConnectionProperties: &routev3.RouteAction_HashPolicy_ConnectionProperties{
					SourceIp: true,
				},
			}
		}
		policies = append(policies, policy)
	}
	outSpec.hashPolicies = policies
	return nil
}
