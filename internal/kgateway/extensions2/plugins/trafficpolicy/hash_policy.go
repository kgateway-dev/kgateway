package trafficpolicy

import (
	"sort"

	"google.golang.org/protobuf/types/known/durationpb"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func hashPolicyForSpec(spec v1alpha1.TrafficPolicySpec, outSpec *trafficPolicySpecIr) {
	if len(spec.HashPolicies) == 0 {
		return
	}
	policies := make([]*envoyroutev3.RouteAction_HashPolicy, 0, len(spec.HashPolicies))
	for _, hashPolicy := range spec.HashPolicies {
		policy := &envoyroutev3.RouteAction_HashPolicy{}
		if hashPolicy.Terminal != nil {
			policy.Terminal = *hashPolicy.Terminal
		}
		switch {
		case hashPolicy.Header != nil:
			policy.PolicySpecifier = &envoyroutev3.RouteAction_HashPolicy_Header_{
				Header: &envoyroutev3.RouteAction_HashPolicy_Header{
					HeaderName: hashPolicy.Header.Name,
				},
			}
		case hashPolicy.Cookie != nil:
			policy.PolicySpecifier = &envoyroutev3.RouteAction_HashPolicy_Cookie_{
				Cookie: &envoyroutev3.RouteAction_HashPolicy_Cookie{
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

				attributes := make([]*envoyroutev3.RouteAction_HashPolicy_CookieAttribute, 0, len(hashPolicy.Cookie.Attributes))
				for _, name := range names {
					attributes = append(attributes, &envoyroutev3.RouteAction_HashPolicy_CookieAttribute{
						Name:  name,
						Value: hashPolicy.Cookie.Attributes[name],
					})
				}
				policy.GetCookie().Attributes = attributes
			}
		case hashPolicy.SourceIP != nil:
			policy.PolicySpecifier = &envoyroutev3.RouteAction_HashPolicy_ConnectionProperties_{
				ConnectionProperties: &envoyroutev3.RouteAction_HashPolicy_ConnectionProperties{
					SourceIp: true,
				},
			}
		}
		policies = append(policies, policy)
	}
	outSpec.hashPolicies = policies
}
