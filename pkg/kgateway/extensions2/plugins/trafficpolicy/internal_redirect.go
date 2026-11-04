package trafficpolicy

import (
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

var _ PolicySubIR = &internalRedirectIR{}

type internalRedirectIR struct {
	policy *envoyroutev3.InternalRedirectPolicy
}

func (a *internalRedirectIR) Equals(other PolicySubIR) bool {
	b, ok := other.(*internalRedirectIR)
	if !ok {
		return false
	}
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return proto.Equal(a.policy, b.policy)
}

func (a *internalRedirectIR) Validate() error {
	if a == nil || a.policy == nil {
		return nil
	}
	return a.policy.Validate()
}

// constructInternalRedirect constructs the internal redirect policy IR from the policy spec.
func constructInternalRedirect(
	spec kgateway.TrafficPolicySpec,
	out *trafficPolicySpecIr,
) {
	if spec.InternalRedirect == nil {
		return
	}

	policy := &envoyroutev3.InternalRedirectPolicy{
		AllowCrossSchemeRedirect: ptr.Deref(spec.InternalRedirect.AllowCrossSchemeRedirect, false),
	}
	if codes := spec.InternalRedirect.RedirectResponseCodes; len(codes) > 0 {
		policy.RedirectResponseCodes = make([]uint32, len(codes))
		for i, code := range codes {
			policy.RedirectResponseCodes[i] = uint32(code) //nolint:gosec // G115: kubebuilder enum validation restricts codes to 301-308
		}
	}
	if headers := spec.InternalRedirect.ResponseHeadersToCopy; len(headers) > 0 {
		policy.ResponseHeadersToCopy = make([]string, len(headers))
		for i, h := range headers {
			policy.ResponseHeadersToCopy[i] = string(h)
		}
	}
	if spec.InternalRedirect.MaxRedirects != nil {
		policy.MaxInternalRedirects = wrapperspb.UInt32(*spec.InternalRedirect.MaxRedirects)
	}

	out.internalRedirect = &internalRedirectIR{
		policy: policy,
	}
}
