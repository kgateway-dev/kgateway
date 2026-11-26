package listenerpolicy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"

	kgwwellknown "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
)

func mergeListenerPolicyAtts(atts ...ir.PolicyAtt) ir.PolicyAtt {
	return policy.MergePolicies(atts, MergePolicies, "" /*no merge settings*/)
}

func listenerPolicyAtt(name string, hierarchicalPriority int, pol *ListenerPolicyIR) ir.PolicyAtt {
	gk := kgwwellknown.ListenerPolicyGVK.GroupKind()
	return ir.PolicyAtt{
		GroupKind:            gk,
		PolicyIr:             pol,
		PolicyRef:            &ir.AttachedPolicyRef{Group: gk.Group, Kind: gk.Kind, Namespace: "default", Name: name},
		HierarchicalPriority: hierarchicalPriority,
	}
}

func TestMergePerPortKeepsOtherPoliciesDefault(t *testing.T) {
	const port = 8443
	gatewayTimeout := durationpb.New(5 * time.Second)
	bufferLimit := uint32(4096)
	newSection := func() *ListenerPolicyIR {
		return &ListenerPolicyIR{perPortPolicy: map[uint32]listenerPolicy{
			port: {perConnectionBufferLimitBytes: &bufferLimit},
		}}
	}
	newGateway := func() *ListenerPolicyIR {
		return &ListenerPolicyIR{defaultPolicy: listenerPolicy{
			transportSocketConnectTimeout: gatewayTimeout,
			http:                          &HttpListenerPolicyIr{useRemoteAddress: new(true)},
		}}
	}
	pass := &listenerPolicyPluginGwPass{}

	cases := []struct {
		name string
		atts func(section, gateway *ListenerPolicyIR) []ir.PolicyAtt
	}{
		{
			name: "section policy at a higher hierarchy",
			atts: func(section, gateway *ListenerPolicyIR) []ir.PolicyAtt {
				return []ir.PolicyAtt{listenerPolicyAtt("section", 1, section), listenerPolicyAtt("gateway", 0, gateway)}
			},
		},
		{
			name: "same hierarchy, section policy first",
			atts: func(section, gateway *ListenerPolicyIR) []ir.PolicyAtt {
				return []ir.PolicyAtt{listenerPolicyAtt("section", 0, section), listenerPolicyAtt("gateway", 0, gateway)}
			},
		},
		{
			name: "same hierarchy, gateway policy first",
			atts: func(section, gateway *ListenerPolicyIR) []ir.PolicyAtt {
				return []ir.PolicyAtt{listenerPolicyAtt("gateway", 0, gateway), listenerPolicyAtt("section", 0, section)}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			section, gateway := newSection(), newGateway()
			merged := mergeListenerPolicyAtts(tc.atts(section, gateway)...)

			cfg := pass.getPolicy(merged.PolicyIr, port)
			assert.Same(t, gatewayTimeout, cfg.transportSocketConnectTimeout, "the gateway default must not be hidden by the section policy's port entry")
			assert.Same(t, &bufferLimit, cfg.perConnectionBufferLimitBytes, "the section policy's port entry must apply")
			require.NotNil(t, cfg.http, "the gateway default's httpSettings must apply on the port")
			assert.Equal(t, new(true), cfg.http.useRemoteAddress)

			gatewayRef := listenerPolicyAtt("gateway", 0, nil).PolicyRef.ID()
			sectionRef := listenerPolicyAtt("section", 0, nil).PolicyRef.ID()
			assert.ElementsMatch(t, []string{gatewayRef}, merged.MergeOrigins.Get("perPortPolicy[8443].transportSocketConnectTimeout"))
			assert.ElementsMatch(t, []string{sectionRef}, merged.MergeOrigins.Get("perPortPolicy[8443].perConnectionBufferLimitBytes"))

			assert.Equal(t, newSection(), section, "merging must not change the section policy IR")
			assert.Equal(t, newGateway(), gateway, "merging must not change the gateway policy IR")
		})
	}
}

func TestMergePerPortReplacesOwnDefault(t *testing.T) {
	// Within one policy, a port entry is used instead of the policy's default.
	bufferLimit := uint32(4096)
	pol := &ListenerPolicyIR{
		defaultPolicy: listenerPolicy{transportSocketConnectTimeout: durationpb.New(5 * time.Second)},
		perPortPolicy: map[uint32]listenerPolicy{8443: {perConnectionBufferLimitBytes: &bufferLimit}},
	}
	merged := mergeListenerPolicyAtts(listenerPolicyAtt("only", 0, pol))

	cfg := (&listenerPolicyPluginGwPass{}).getPolicy(merged.PolicyIr, 8443)
	assert.Nil(t, cfg.transportSocketConnectTimeout, "a policy's own default does not apply on a port it has an entry for")
	assert.Same(t, &bufferLimit, cfg.perConnectionBufferLimitBytes)

	other := (&listenerPolicyPluginGwPass{}).getPolicy(merged.PolicyIr, 9000)
	assert.Equal(t, durationpb.New(5*time.Second), other.transportSocketConnectTimeout, "ports without an entry use the default")
}

func TestMergePerPortHigherPriorityDefaultWins(t *testing.T) {
	// The higher-priority policy's default beats the lower-priority policy's port entry for the
	// fields both set, and the port entry fills the rest.
	high := &ListenerPolicyIR{defaultPolicy: listenerPolicy{transportSocketConnectTimeout: durationpb.New(10 * time.Second)}}
	low := &ListenerPolicyIR{perPortPolicy: map[uint32]listenerPolicy{8443: {
		transportSocketConnectTimeout: durationpb.New(5 * time.Second),
		perConnectionBufferLimitBytes: new(uint32(4096)),
	}}}
	merged := mergeListenerPolicyAtts(listenerPolicyAtt("high", 1, high), listenerPolicyAtt("low", 0, low))

	cfg := (&listenerPolicyPluginGwPass{}).getPolicy(merged.PolicyIr, 8443)
	assert.Equal(t, durationpb.New(10*time.Second), cfg.transportSocketConnectTimeout)
	assert.Equal(t, new(uint32(4096)), cfg.perConnectionBufferLimitBytes)
}
