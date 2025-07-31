package trafficpolicy

import (
	"sort"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

type hashPolicyIR struct {
	policies []*envoyroutev3.RouteAction_HashPolicy
}

var _ PolicySubIR = &hashPolicyIR{}

func (h *hashPolicyIR) Equals(other PolicySubIR) bool {
	otherHashPolicy, ok := other.(*hashPolicyIR)
	if !ok {
		return false
	}
	if h == nil && otherHashPolicy == nil {
		return true
	}
	if h == nil || otherHashPolicy == nil {
		return false
	}
	if len(h.policies) != len(otherHashPolicy.policies) {
		return false
	}
	for i, policy := range h.policies {
		if !proto.Equal(policy, otherHashPolicy.policies[i]) {
			return false
		}
	}
	return true
}

func (h *hashPolicyIR) Validate() error {
	if h == nil {
		return nil
	}
	for _, policy := range h.policies {
		if err := policy.ValidateAll(); err != nil {
			return err
		}
	}
	return nil
}

// constructHashPolicy constructs the hash policy IR from the policy specification.
func constructHashPolicy(spec v1alpha1.TrafficPolicySpec, outSpec *trafficPolicySpecIr) {
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
	outSpec.hashPolicies = &hashPolicyIR{
		policies: policies,
	}
}
