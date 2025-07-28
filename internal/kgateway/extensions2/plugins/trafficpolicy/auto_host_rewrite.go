package trafficpolicy

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

type autoHostRewriteIR struct {
	enabled *wrapperspb.BoolValue
}

func (a *autoHostRewriteIR) Equals(other *autoHostRewriteIR) bool {
	if a == nil && other == nil {
		return true
	}
	if a == nil || other == nil {
		return false
	}
	return proto.Equal(a.enabled, other.enabled)
}

// applyAutoHostRewrite translates the auto host rewrite spec into an envoy auto host rewrite policy and stores it in the traffic policy IR
func applyAutoHostRewrite(spec v1alpha1.TrafficPolicySpec, out *trafficPolicySpecIr) {
	if spec.AutoHostRewrite == nil {
		return
	}
	out.autoHostRewrite = &autoHostRewriteIR{
		enabled: wrapperspb.Bool(*spec.AutoHostRewrite),
	}
}
