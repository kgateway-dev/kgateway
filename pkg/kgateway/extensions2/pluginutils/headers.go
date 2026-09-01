package pluginutils

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	mutation_rulesv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"istio.io/istio/pkg/kube/krt"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	sharedv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
)

var (
	ErrUnsupportedRemoveHeaderMutation = errors.New("remove mutation cannot be converted to append action")
	ErrUnknownHeaderMutation           = errors.New("unknown header mutation")
)

func ConvertMutations(filter *gwv1.HTTPHeaderFilter) (mutations []*mutation_rulesv3.HeaderMutation) {
	if filter == nil {
		return nil
	}

	if len(filter.Add) == 0 && len(filter.Set) == 0 && len(filter.Remove) == 0 {
		return nil
	}

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

func ConvertHeaderFilter(
	krtctx krt.HandlerContext,
	from krtcollections.From,
	secrets *krtcollections.SecretIndex,
	filter *sharedv1alpha1.HTTPHeaderFilter,
) (*gwv1.HTTPHeaderFilter, error) {
	if filter == nil {
		return nil, nil
	}

	if len(filter.Add) == 0 && len(filter.Set) == 0 && len(filter.Remove) == 0 {
		return nil, nil
	}

	gwFilter := &gwv1.HTTPHeaderFilter{}
	for _, h := range filter.Add {
		resolved, err := resolveHeader(krtctx, from, secrets, h)
		if err != nil {
			if h.Name != nil {
				return nil, fmt.Errorf("failed to resolve header '%s': %w", *h.Name, err)
			}
			return nil, fmt.Errorf("failed to resolve header value(s): %w", err)
		}
		for _, r := range resolved {
			gwFilter.Add = append(gwFilter.Add, r)
		}
	}

	for _, h := range filter.Set {
		resolved, err := resolveHeader(krtctx, from, secrets, h)
		if err != nil {
			if h.Name != nil {
				return nil, fmt.Errorf("failed to resolve header '%s': %w", *h.Name, err)
			}
			return nil, fmt.Errorf("failed to resolve header value(s): %w", err)
		}
		for _, r := range resolved {
			gwFilter.Set = append(gwFilter.Set, r)
		}
	}

	gwFilter.Remove = filter.Remove

	return gwFilter, nil
}

// resolveHeader resolves an HTTPHeader entry into one or more gwv1.HTTPHeader pairs.
// When both name and key are absent on a secretRef entry, all Secret data entries are returned.
func resolveHeader(
	krtctx krt.HandlerContext,
	from krtcollections.From,
	secrets *krtcollections.SecretIndex,
	h sharedv1alpha1.HTTPHeader,
) ([]gwv1.HTTPHeader, error) {
	if h.Value != nil {
		return []gwv1.HTTPHeader{{Name: *h.Name, Value: *h.Value}}, nil
	}

	ref := h.SecretRef
	ns := gwv1.Namespace(from.Namespace)
	if ref.Namespace != nil {
		ns = *ref.Namespace
	}
	secretRef := gwv1.SecretObjectReference{
		Name:      ref.Name,
		Namespace: &ns,
	}

	secret, err := secrets.GetSecret(krtctx, from, secretRef)
	if err != nil {
		return nil, fmt.Errorf("secret %s/%s: %w", ns, ref.Name, err)
	}

	// Both name and key absent: inject every entry in the Secret as a header.
	if ref.Key == nil && h.Name == nil {
		pairs := make([]gwv1.HTTPHeader, 0, len(secret.Data))
		for k, v := range secret.Data {
			pairs = append(pairs, gwv1.HTTPHeader{Name: gwv1.HTTPHeaderName(k), Value: string(v)})
		}
		slices.SortFunc(pairs, func(a, b gwv1.HTTPHeader) int {
			return cmp.Compare(string(a.Name), string(b.Name))
		})
		return pairs, nil
	}

	// Determine which key to look up: explicit key, or fall back to header name.
	// At this point at least one of ref.Key or h.Name is non-nil (both-nil handled above).
	var key string
	if ref.Key != nil {
		key = *ref.Key
	} else {
		key = string(*h.Name)
	}

	data, ok := secret.Data[key]
	if !ok {
		return nil, fmt.Errorf("secret %s/%s does not contain key %q", ns, ref.Name, key)
	}

	// Determine header name: explicit name, or fall back to key.
	headerName := gwv1.HTTPHeaderName(key)
	if h.Name != nil {
		headerName = *h.Name
	}
	return []gwv1.HTTPHeader{{Name: headerName, Value: string(data)}}, nil
}

func ConvertMutationsToOptions(mutations []*mutation_rulesv3.HeaderMutation) (options []*envoycorev3.HeaderValueOption, err error) {
	for _, m := range mutations {
		switch a := m.Action.(type) {
		case *mutation_rulesv3.HeaderMutation_Append:
			options = append(options, a.Append)
		case *mutation_rulesv3.HeaderMutation_Remove:
			return nil, ErrUnsupportedRemoveHeaderMutation
		default:
			return nil, ErrUnknownHeaderMutation
		}
	}

	return options, nil
}
