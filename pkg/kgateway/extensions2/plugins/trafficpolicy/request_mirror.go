package trafficpolicy

import (
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

type requestMirrorIR struct {
	disableShadowHostSuffixAppend *bool
}

var _ PolicySubIR = &requestMirrorIR{}

func (r *requestMirrorIR) Equals(other PolicySubIR) bool {
	otherRequestMirror, ok := other.(*requestMirrorIR)
	if !ok {
		return false
	}
	if r == nil || otherRequestMirror == nil {
		return r == nil && otherRequestMirror == nil
	}
	if r.disableShadowHostSuffixAppend == nil || otherRequestMirror.disableShadowHostSuffixAppend == nil {
		return r.disableShadowHostSuffixAppend == nil && otherRequestMirror.disableShadowHostSuffixAppend == nil
	}
	return *r.disableShadowHostSuffixAppend == *otherRequestMirror.disableShadowHostSuffixAppend
}

// Validate performs validation on the request mirror component. The API schema
// validates that at least one setting is present, so no IR validation is needed.
func (r *requestMirrorIR) Validate() error { return nil }

// constructRequestMirror constructs the request mirror policy IR from the policy specification.
func constructRequestMirror(spec kgateway.TrafficPolicySpec, out *trafficPolicySpecIr) {
	if spec.RequestMirror == nil {
		return
	}

	out.requestMirror = &requestMirrorIR{}
	if spec.RequestMirror.DisableShadowHostSuffixAppend != nil {
		disableShadowHostSuffixAppend := *spec.RequestMirror.DisableShadowHostSuffixAppend
		out.requestMirror.disableShadowHostSuffixAppend = &disableShadowHostSuffixAppend
	}
}

func (p *trafficPolicyPluginGwPass) applyRequestMirror(requestMirror *requestMirrorIR, out *envoyroutev3.Route) {
	if requestMirror == nil || requestMirror.disableShadowHostSuffixAppend == nil || out == nil || out.GetRoute() == nil {
		return
	}

	// Route policies run before listener and Gateway policies. Track explicitly configured routes so a
	// less-specific policy cannot overwrite an explicit false. We track it out-of-band because the Envoy
	// field is a plain bool with no unset state to gate on, unlike the other route-action fields.
	if p.requestMirrorConfigured == nil {
		p.requestMirrorConfigured = make(map[*envoyroutev3.Route]struct{})
	}
	if _, configured := p.requestMirrorConfigured[out]; configured {
		return
	}
	p.requestMirrorConfigured[out] = struct{}{}

	for _, mirror := range out.GetRoute().GetRequestMirrorPolicies() {
		if mirror != nil {
			mirror.DisableShadowHostSuffixAppend = *requestMirror.disableShadowHostSuffixAppend
		}
	}
}
