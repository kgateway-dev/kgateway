package sharedproto

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
)

func withAssertions(t *testing.T, enabled bool) {
	t.Helper()
	prev := AssertImmutability
	AssertImmutability = enabled
	t.Cleanup(func() { AssertImmutability = prev })
}

func TestResourceWithTTL_PanicsOnMutation(t *testing.T) {
	withAssertions(t, true)
	cluster := &envoyclusterv3.Cluster{Name: "shared"}
	s := Wrap(cluster)

	require.NotPanics(t, func() { s.ResourceWithTTL() },
		"an unmutated proto must pass verification")

	// The canonical nasty mistake: mutating the wrapped proto through a
	// retained raw pointer.
	cluster.OutlierDetection = &envoyclusterv3.OutlierDetection{}

	require.Panics(t, func() { s.ResourceWithTTL() },
		"a mutated shared proto must trip the assertion")
}

func TestResourceWithTTL_SkipsUncaptured(t *testing.T) {
	withAssertions(t, true)
	cluster := &envoyclusterv3.Cluster{Name: "fixture"}
	s := WrapPrehashed(cluster, 0)
	cluster.OutlierDetection = &envoyclusterv3.OutlierDetection{}

	require.NotPanics(t, func() { s.ResourceWithTTL() },
		"hash 0 means not captured (error-path rows, flag off at wrap) and must be skipped")
	require.Same(t, cluster, s.ResourceWithTTL().Resource,
		"the wrapped proto must be handed to the snapshot unchanged")
}

func TestWrap_RespectsFlag(t *testing.T) {
	cluster := &envoyclusterv3.Cluster{Name: "c"}

	withAssertions(t, false)
	require.Zero(t, Wrap(cluster).hash, "capture must be free when assertions are disabled")
	require.Zero(t, WrapPrehashed(cluster, 42).hash)

	withAssertions(t, true)
	require.Equal(t, utils.HashProto(cluster), Wrap(cluster).hash,
		"Wrap must capture the content hash while assertions are enabled")
	require.Equal(t, uint64(42), WrapPrehashed(cluster, 42).hash)
}

func TestCloneIsIndependent(t *testing.T) {
	withAssertions(t, true)
	cluster := &envoyclusterv3.Cluster{Name: "shared"}
	s := Wrap(cluster)

	clone := s.Clone()
	require.NotSame(t, cluster, clone)
	clone.OutlierDetection = &envoyclusterv3.OutlierDetection{}

	require.NotPanics(t, func() { s.ResourceWithTTL() },
		"mutating a Clone must not affect the shared proto")
	assert.Nil(t, cluster.GetOutlierDetection())
}

func TestIdentityHelpers(t *testing.T) {
	a := &envoyclusterv3.Cluster{Name: "a"}
	b := &envoyclusterv3.Cluster{Name: "a"} // equal content, distinct instance

	sa, sb := Wrap(a), Wrap(b)
	require.True(t, Same(sa, Wrap(a)), "Same must report aliasing of one instance")
	require.False(t, Same(sa, sb), "Same must be identity, not content equality")
	require.True(t, sa.Is(a))
	require.False(t, sa.Is(b))
}

func TestIsNil(t *testing.T) {
	var zero Shared[*envoyclusterv3.Cluster]
	require.True(t, zero.IsNil(), "zero-value wrapper carries no proto")
	require.True(t, Wrap[*envoyclusterv3.Cluster](nil).IsNil(), "wrapped typed-nil is nil")
	require.False(t, Wrap(&envoyclusterv3.Cluster{}).IsNil())
}
