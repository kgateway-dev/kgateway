package pluginutils

import (
	mutation_rulesv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func ConvertMutations(filter *gwv1.HTTPHeaderFilter) []*mutation_rulesv3.HeaderMutation {
	if filter == nil {
		return nil
	}

	if len(filter.Add) == 0 && len(filter.Set) == 0 && len(filter.Remove) == 0 {
		return nil
	}

	var mutations []*mutation_rulesv3.HeaderMutation

	for _, h := range filter.Add {
		mutations = append(mutations, &mutation_rulesv3.HeaderMutation{
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

	for _, h := range filter.Set {
		mutations = append(mutations, &mutation_rulesv3.HeaderMutation{
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

	for _, h := range filter.Remove {
		mutations = append(mutations, &mutation_rulesv3.HeaderMutation{
			Action: &mutation_rulesv3.HeaderMutation_Remove{
				Remove: h,
			},
		})
	}
	return mutations
}
