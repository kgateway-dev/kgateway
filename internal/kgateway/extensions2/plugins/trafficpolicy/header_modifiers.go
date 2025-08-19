package trafficpolicy

import (
	mutation_rulesv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	header_mutationv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

const (
	headerMutationFilterName = "envoy.extensions.filters.http.header_mutation"
)

type headerModifiersIR struct {
	policy *header_mutationv3.HeaderMutationPerRoute
}

var _ PolicySubIR = &headerModifiersIR{}

func (hm *headerModifiersIR) Equals(other PolicySubIR) bool {
	otherheaderModifiers, ok := other.(*headerModifiersIR)
	if !ok {
		return false
	}
	if hm == nil || otherheaderModifiers == nil {
		return hm == nil && otherheaderModifiers == nil
	}

	return proto.Equal(hm.policy, otherheaderModifiers.policy)
}

func (hm *headerModifiersIR) Validate() error {
	if hm == nil || hm.policy == nil {
		return nil
	}

	return hm.policy.Validate()
}

// constructHeaderModifiers constructs the headerModifiers policy IR from the policy specification.
func constructHeaderModifiers(spec v1alpha1.TrafficPolicySpec, out *trafficPolicySpecIr) error {
	if spec.HeaderModifiers == nil {
		return nil
	}

	p := buildHeaderModifiersPolicy(spec.HeaderModifiers)

	out.headerModifiers = &headerModifiersIR{
		policy: p,
	}

	return nil
}

// handleHeaderModifiers adds header modifier filters.
func (p *trafficPolicyPluginGwPass) handleHeaderModifiers(fcn string, typedFilterConfig *ir.TypedFilterConfigMap, ir *headerModifiersIR) {
	if ir == nil {
		return
	}

	typedFilterConfig.AddTypedConfig(headerMutationFilterName, ir.policy)

	// Add a filter to the chain. When having a header mutation for a route we need to also have a
	// empty header mutation filter in the chain, otherwise it will be ignored.
	// If there is also header mutation filter for the listener, it will not override this one.
	if p.headerMutationInChain == nil {
		p.headerMutationInChain = make(map[string]*header_mutationv3.HeaderMutationPerRoute)
	}

	if _, ok := p.headerMutationInChain[fcn]; !ok {
		p.headerMutationInChain[fcn] = &header_mutationv3.HeaderMutationPerRoute{}
	}
}

// buildHeaderModifiersPolicy converts a TrafficPolicy HeaderModifiersPolicy into an Envoy HeaderMutationPerRoute.
func buildHeaderModifiersPolicy(
	spec *v1alpha1.HeaderModifiers,
) *header_mutationv3.HeaderMutationPerRoute {
	policy := &header_mutationv3.HeaderMutationPerRoute{}
	policy.Mutations = &header_mutationv3.Mutations{}

	if spec.Request != nil {
		for _, h := range spec.Request.Add {
			policy.Mutations.RequestMutations = append(policy.Mutations.RequestMutations, &mutation_rulesv3.HeaderMutation{
				Action: &mutation_rulesv3.HeaderMutation_Append{
					Append: &envoycorev3.HeaderValueOption{
						Header: &envoycorev3.HeaderValue{
							Key:   string(h.Name),
							Value: h.Value,
						},
						AppendAction: envoycorev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
					},
				},
			})
		}

		for _, h := range spec.Request.Set {
			policy.Mutations.RequestMutations = append(policy.Mutations.RequestMutations, &mutation_rulesv3.HeaderMutation{
				Action: &mutation_rulesv3.HeaderMutation_Append{
					Append: &envoycorev3.HeaderValueOption{
						Header: &envoycorev3.HeaderValue{
							Key:   string(h.Name),
							Value: h.Value,
						},
						AppendAction: envoycorev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
					},
				},
			})
		}

		for _, h := range spec.Request.Remove {
			policy.Mutations.RequestMutations = append(policy.Mutations.RequestMutations, &mutation_rulesv3.HeaderMutation{
				Action: &mutation_rulesv3.HeaderMutation_Remove{
					Remove: h,
				},
			})
		}
	}

	if spec.Response != nil {
		for _, h := range spec.Response.Add {
			policy.Mutations.ResponseMutations = append(policy.Mutations.ResponseMutations, &mutation_rulesv3.HeaderMutation{
				Action: &mutation_rulesv3.HeaderMutation_Append{
					Append: &envoycorev3.HeaderValueOption{
						Header: &envoycorev3.HeaderValue{
							Key:   string(h.Name),
							Value: h.Value,
						},
						AppendAction: envoycorev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
					},
				},
			})
		}

		for _, h := range spec.Response.Set {
			policy.Mutations.ResponseMutations = append(policy.Mutations.ResponseMutations, &mutation_rulesv3.HeaderMutation{
				Action: &mutation_rulesv3.HeaderMutation_Append{
					Append: &envoycorev3.HeaderValueOption{
						Header: &envoycorev3.HeaderValue{
							Key:   string(h.Name),
							Value: h.Value,
						},
						AppendAction: envoycorev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
					},
				},
			})
		}

		for _, h := range spec.Response.Remove {
			policy.Mutations.ResponseMutations = append(policy.Mutations.ResponseMutations, &mutation_rulesv3.HeaderMutation{
				Action: &mutation_rulesv3.HeaderMutation_Remove{
					Remove: h,
				},
			})
		}
	}

	if len(policy.Mutations.RequestMutations) == 0 && len(policy.Mutations.ResponseMutations) == 0 {
		policy.Mutations = nil
	}

	return policy
}
