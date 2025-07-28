package trafficpolicy

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

type AutoHostRewriteIR struct {
	enabled *wrapperspb.BoolValue
}

func (a *AutoHostRewriteIR) Equals(other *AutoHostRewriteIR) bool {
	if a == nil && other == nil {
		return true
	}
	if a == nil || other == nil {
		return false
	}
	return proto.Equal(a.enabled, other.enabled)
}

func (a *AutoHostRewriteIR) AutoHostRewrite() *wrapperspb.BoolValue {
	if a == nil {
		return nil
	}
	return a.enabled
}

// autoHostRewriteForSpec translates the auto host rewrite spec into an envoy auto host rewrite policy and stores it in the traffic policy IR
func autoHostRewriteForSpec(spec v1alpha1.TrafficPolicySpec, out *trafficPolicySpecIr) {
	if spec.AutoHostRewrite == nil {
		return
	}
	out.autoHostRewrite = &AutoHostRewriteIR{
		enabled: wrapperspb.Bool(*spec.AutoHostRewrite),
	}
}
