package trafficpolicy

import (
	"fmt"

	header_mutationv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
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
	from := krtcollections.From{
		GroupKind: wellknown.TrafficPolicyGVK.GroupKind(),
		Namespace: policy.Namespace,
	}

	p := &header_mutationv3.HeaderMutationPerRoute{
		Mutations: &header_mutationv3.Mutations{},
	}

	gwReqFilter, err := pluginutils.ConvertHeaderFilter(krtctx, from, secrets, spec.Request)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	reqMutations := pluginutils.ConvertMutations(gwReqFilter)
	p.Mutations.RequestMutations = append(p.Mutations.RequestMutations, reqMutations...)

	gwRespFilter, err := pluginutils.ConvertHeaderFilter(krtctx, from, secrets, spec.Response)
	if err != nil {
		return fmt.Errorf("response: %w", err)
	}
	respMutations := pluginutils.ConvertMutations(gwRespFilter)
	p.Mutations.ResponseMutations = append(p.Mutations.ResponseMutations, respMutations...)

	if len(p.Mutations.RequestMutations) == 0 && len(p.Mutations.ResponseMutations) == 0 {
		p.Mutations = nil
	}

	out.headerModifiers = &headerModifiersIR{policy: p}
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
