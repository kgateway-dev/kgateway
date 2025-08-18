package policy

import (
	mutation_rulesv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	header_mutationv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

// BuildHeaderModifiersPolicy converts a TrafficPolicy HeaderModifiersPolicy into an Envoy HeaderMutationPerRoute.
func BuildHeaderModifiersPolicy(
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
