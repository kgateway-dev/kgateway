package trafficpolicy

import (
	"fmt"

	mutation_rulesv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	header_mutationv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	sharedv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
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
	p := buildHeaderModifiersPolicy(spec)

	if p.Mutations == nil {
		p.Mutations = &header_mutationv3.Mutations{}
	}

	from := krtcollections.From{
		GroupKind: wellknown.TrafficPolicyGVK.GroupKind(),
		Namespace: policy.Namespace,
	}

	reqMutations, err := buildHeaderMutationsFromSecretMappings(krtctx, from, secrets, spec.RequestHeadersFromSecret)
	if err != nil {
		return fmt.Errorf("requestHeadersFromSecret: %w", err)
	}
	p.Mutations.RequestMutations = append(p.Mutations.RequestMutations, reqMutations...)

	respMutations, err := buildHeaderMutationsFromSecretMappings(krtctx, from, secrets, spec.ResponseHeadersFromSecret)
	if err != nil {
		return fmt.Errorf("responseHeadersFromSecret: %w", err)
	}
	p.Mutations.ResponseMutations = append(p.Mutations.ResponseMutations, respMutations...)

	if len(p.Mutations.RequestMutations) == 0 && len(p.Mutations.ResponseMutations) == 0 {
		p.Mutations = nil
	}

	out.headerModifiers = &headerModifiersIR{
		policy: p,
	}
	return nil
}

// buildHeaderMutationsFromSecretMappings resolves secret-backed header values and returns the
// corresponding Envoy header mutations. Each mapping specifies the secret reference, the key
// within the secret, and the header name to inject. Cross-namespace references are validated
// via ReferenceGrant.
func buildHeaderMutationsFromSecretMappings(
	krtctx krt.HandlerContext,
	from krtcollections.From,
	secrets *krtcollections.SecretIndex,
	mappings []sharedv1alpha1.SecretHeaderMapping,
) ([]*mutation_rulesv3.HeaderMutation, error) {
	if len(mappings) == 0 {
		return nil, nil
	}

	var mutations []*mutation_rulesv3.HeaderMutation
	for _, m := range mappings {
		secretRef := gwv1.SecretObjectReference{
			Name: m.SecretRef.Name,
		}
		if m.SecretRef.Namespace != nil {
			secretRef.Namespace = m.SecretRef.Namespace
		}

		secret, err := secrets.GetSecret(krtctx, from, secretRef)
		if err != nil {
			return nil, fmt.Errorf("secret %s: %w", m.SecretRef.Name, err)
		}

		value, ok := secret.Data[m.Key]
		if !ok {
			return nil, fmt.Errorf("secret %s does not contain key %q", m.SecretRef.Name, m.Key)
		}

		mutations = append(mutations, &mutation_rulesv3.HeaderMutation{
			Action: &mutation_rulesv3.HeaderMutation_Append{
				Append: &envoycorev3.HeaderValueOption{
					Header: &envoycorev3.HeaderValue{
						Key:   string(m.Header),
						Value: string(value),
					},
					AppendAction: envoycorev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
				},
			},
		})
	}
	return mutations, nil
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
	spec *sharedv1alpha1.HeaderModifiers,
) *header_mutationv3.HeaderMutationPerRoute {
	policy := &header_mutationv3.HeaderMutationPerRoute{}
	policy.Mutations = &header_mutationv3.Mutations{}

	policy.Mutations.RequestMutations = append(policy.Mutations.RequestMutations, pluginutils.ConvertMutations(spec.Request)...)
	policy.Mutations.ResponseMutations = append(policy.Mutations.ResponseMutations, pluginutils.ConvertMutations(spec.Response)...)

	// Secret-based mutations are appended by constructHeaderModifiers after this call; do not nil
	// Mutations here — that is handled by the caller once all sources have been processed.

	return policy
}
