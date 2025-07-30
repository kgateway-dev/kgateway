package trafficpolicy

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

type autoHostRewriteIR struct {
	enabled *wrapperspb.BoolValue
}

var _ PolicySubIR = &autoHostRewriteIR{}

func (a *autoHostRewriteIR) Equals(other PolicySubIR) bool {
	otherAutoHostRewrite, ok := other.(*autoHostRewriteIR)
	if !ok {
		return false
	}
	if a == nil && otherAutoHostRewrite == nil {
		return true
	}
	if a == nil || otherAutoHostRewrite == nil {
		return false
	}
	return proto.Equal(a.enabled, otherAutoHostRewrite.enabled)
}

// Validate performs validation on the auto host rewrite component. No validation is
// needed as it's a single bool field.
func (a *autoHostRewriteIR) Validate() error { return nil }

// constructAutoHostRewrite constructs the auto host rewrite policy IR from the policy specification.
func constructAutoHostRewrite(spec v1alpha1.TrafficPolicySpec, out *trafficPolicySpecIr) {
	if spec.AutoHostRewrite == nil {
		return
	}
	out.autoHostRewrite = &autoHostRewriteIR{
		enabled: wrapperspb.Bool(*spec.AutoHostRewrite),
	}
}
