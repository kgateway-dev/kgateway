package trafficpolicy

import (
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

type requestMirrorIR struct {
	disableShadowHostSuffixAppend *bool
	hostRewriteLiteral            *string
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
	return ptr.Equal(r.disableShadowHostSuffixAppend, otherRequestMirror.disableShadowHostSuffixAppend) &&
		ptr.Equal(r.hostRewriteLiteral, otherRequestMirror.hostRewriteLiteral)
}

// Validate performs validation on the request mirror component. The API schema
// validates that at least one setting is present, so no IR validation is needed.
func (r *requestMirrorIR) Validate() error { return nil }

// constructRequestMirror constructs the request mirror policy IR from the policy specification.
func constructRequestMirror(spec kgateway.TrafficPolicySpec, out *trafficPolicySpecIr) {
	if spec.RequestMirror == nil {
		return
	}

	// Copy into fresh pointers so the IR does not alias the policy spec.
	mirror := &requestMirrorIR{}
	if spec.RequestMirror.DisableShadowHostSuffixAppend != nil {
		mirror.disableShadowHostSuffixAppend = new(*spec.RequestMirror.DisableShadowHostSuffixAppend)
	}
	if spec.RequestMirror.HostRewriteLiteral != nil {
		mirror.hostRewriteLiteral = new(*spec.RequestMirror.HostRewriteLiteral)
	}
	out.requestMirror = mirror
}

func (p *trafficPolicyPluginGwPass) applyRequestMirror(requestMirror *requestMirrorIR, out *envoyroutev3.Route) {
	if requestMirror == nil ||
		(requestMirror.disableShadowHostSuffixAppend == nil && requestMirror.hostRewriteLiteral == nil) ||
		out == nil || out.GetRoute() == nil {
		return
	}

	// Route policies run before listener and Gateway policies. Track routes (by name) that a policy has
	// already configured so a less-specific policy cannot overwrite a more-specific requestMirror block.
	// The whole block is owned by the most-specific policy; we track it out-of-band because the Envoy
	// fields have no unset state to gate on, unlike the other route-action fields.
	routeName := out.GetName()
	if p.requestMirrorConfigured == nil {
		p.requestMirrorConfigured = make(map[string]struct{})
	}
	if _, configured := p.requestMirrorConfigured[routeName]; configured {
		return
	}
	p.requestMirrorConfigured[routeName] = struct{}{}

	for _, mirror := range out.GetRoute().GetRequestMirrorPolicies() {
		if mirror == nil {
			continue
		}
		if requestMirror.disableShadowHostSuffixAppend != nil {
			mirror.DisableShadowHostSuffixAppend = *requestMirror.disableShadowHostSuffixAppend
		}
		if requestMirror.hostRewriteLiteral != nil {
			mirror.HostRewriteLiteral = *requestMirror.hostRewriteLiteral
		}
	}
}
