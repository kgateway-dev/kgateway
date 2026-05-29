package trafficpolicy

import (
	"fmt"
	"sort"

	mutation_rulesv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	header_mutationv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	sharedv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
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
// It resolves any secret-backed header values via the secrets index (ReferenceGrant-enforced).
func constructHeaderModifiers(
	krtctx krt.HandlerContext,
	policy *kgateway.TrafficPolicy,
	secrets *krtcollections.SecretIndex,
	out *trafficPolicySpecIr,
) error {
	if policy.Spec.HeaderModifiers == nil {
		return nil
	}

	spec := policy.Spec.HeaderModifiers
	from := krtcollections.From{
		GroupKind: wellknown.TrafficPolicyGVK.GroupKind(),
		Namespace: policy.Namespace,
	}

	p := &header_mutationv3.HeaderMutationPerRoute{
		Mutations: &header_mutationv3.Mutations{},
	}

	reqMutations, err := buildHTTPHeaderFilterMutations(krtctx, from, secrets, spec.Request)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	p.Mutations.RequestMutations = append(p.Mutations.RequestMutations, reqMutations...)

	respMutations, err := buildHTTPHeaderFilterMutations(krtctx, from, secrets, spec.Response)
	if err != nil {
		return fmt.Errorf("response: %w", err)
	}
	p.Mutations.ResponseMutations = append(p.Mutations.ResponseMutations, respMutations...)

	if len(p.Mutations.RequestMutations) == 0 && len(p.Mutations.ResponseMutations) == 0 {
		p.Mutations = nil
	}

	out.headerModifiers = &headerModifiersIR{policy: p}
	return nil
}

// buildHTTPHeaderFilterMutations converts an HTTPHeaderFilter into Envoy header mutations,
// resolving any secret-backed values via the secrets index.
func buildHTTPHeaderFilterMutations(
	krtctx krt.HandlerContext,
	from krtcollections.From,
	secrets *krtcollections.SecretIndex,
	filter *sharedv1alpha1.HTTPHeaderFilter,
) ([]*mutation_rulesv3.HeaderMutation, error) {
	if filter == nil {
		return nil, nil
	}

	var mutations []*mutation_rulesv3.HeaderMutation

	appendMutations := func(headers []sharedv1alpha1.HTTPHeader, action envoycorev3.HeaderValueOption_HeaderAppendAction) error {
		for _, h := range headers {
			pairs, err := resolveHeader(krtctx, from, secrets, h)
			if err != nil {
				return err
			}
			for _, p := range pairs {
				mutations = append(mutations, headerMutation(string(p.Name), p.Value, action))
			}
		}
		return nil
	}

	if err := appendMutations(filter.Set, envoycorev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD); err != nil {
		return nil, err
	}
	if err := appendMutations(filter.Add, envoycorev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD); err != nil {
		return nil, err
	}

	for _, name := range filter.Remove {
		mutations = append(mutations, &mutation_rulesv3.HeaderMutation{
			Action: &mutation_rulesv3.HeaderMutation_Remove{
				Remove: name,
			},
		})
	}

	return mutations, nil
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
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].Name < pairs[j].Name })
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

func headerMutation(name, value string, action envoycorev3.HeaderValueOption_HeaderAppendAction) *mutation_rulesv3.HeaderMutation {
	return &mutation_rulesv3.HeaderMutation{
		Action: &mutation_rulesv3.HeaderMutation_Append{
			Append: &envoycorev3.HeaderValueOption{
				Header: &envoycorev3.HeaderValue{
					Key:   name,
					Value: value,
				},
				AppendAction: action,
			},
		},
	}
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
