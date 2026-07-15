package listenerpolicy

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
	"github.com/kgateway-dev/kgateway/v2/test/testutils/equalstest"
)

// baseHarnessTcpListenerPolicyIr returns a fully-populated TcpListenerPolicyIr so
// that every field can be mutated to a distinguishable value by a Case below.
func baseHarnessTcpListenerPolicyIr() *TcpListenerPolicyIr {
	return &TcpListenerPolicyIr{
		accessLogConfig: []proto.Message{wrapperspb.String("access-log")},
		accessLogPolicies: []kgateway.AccessLog{
			{FileSink: &kgateway.FileSink{Path: "/dev/stdout"}},
		},
	}
}

// TestHarnessTcpListenerPolicyIrEquals exercises every field of TcpListenerPolicyIr,
// so that adding a field without a matching Equals comparison fails the test
// instead of silently dropping Envoy config updates.
func TestHarnessTcpListenerPolicyIrEquals(t *testing.T) {
	cases := []equalstest.Case[*TcpListenerPolicyIr]{
		{
			Field: "accessLogConfig",
			Mutate: func(d **TcpListenerPolicyIr) {
				(*d).accessLogConfig = []proto.Message{wrapperspb.String("access-log-2")}
			},
		},
		{
			Field: "accessLogPolicies",
			Mutate: func(d **TcpListenerPolicyIr) {
				(*d).accessLogPolicies = []kgateway.AccessLog{{FileSink: &kgateway.FileSink{Path: "/dev/stderr"}}}
			},
		},
	}

	equalstest.Run(
		t,
		baseHarnessTcpListenerPolicyIr,
		func(a, b *TcpListenerPolicyIr) bool { return a.Equals(b) },
		cases,
		nil,
		equalstest.IncludeUnexported(),
	)
}

// TestMergeTcpPolicies verifies that TcpSettings.AccessLog is merged from p2 into
// p1 when p1 has no access logs of its own (augmented merge), and that origins are
// tracked so status reflects where the merged config came from.
func TestMergeTcpPolicies(t *testing.T) {
	p1 := &TcpListenerPolicyIr{}
	p2 := &TcpListenerPolicyIr{
		accessLogConfig: []proto.Message{wrapperspb.String("access-log")},
		accessLogPolicies: []kgateway.AccessLog{
			{FileSink: &kgateway.FileSink{Path: "/dev/stdout"}},
		},
	}

	p2Ref := &ir.AttachedPolicyRef{Name: "tcp-access-log", Namespace: "default"}
	mergeOrigins := ir.MergeOrigins{}

	MergeTcpPolicies(
		"default.tcpSettings.",
		p1, p2, p2Ref, nil,
		policy.MergeOptions{Strategy: policy.AugmentedDeepMerge},
		mergeOrigins,
	)

	if len(p1.accessLogConfig) != 1 {
		t.Fatalf("expected accessLogConfig to be merged into p1, got %d entries", len(p1.accessLogConfig))
	}
	if len(p1.accessLogPolicies) != 1 {
		t.Fatalf("expected accessLogPolicies to be merged into p1, got %d entries", len(p1.accessLogPolicies))
	}
	if got := mergeOrigins["default.tcpSettings.accessLog"]; got == nil {
		t.Fatalf("expected merge origin to be tracked for default.tcpSettings.accessLog")
	}
}
