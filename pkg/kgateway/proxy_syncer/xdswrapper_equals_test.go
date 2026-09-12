package proxy_syncer

import (
	"testing"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/stretchr/testify/assert"
)

// TestXdsSnapWrapperEquals_ComparesGapClassification pins the regression where
// a wrapper's deferred->ready transition was invisible to KRT: a synthesized
// empty CLA and a derived-but-empty CLA are byte-identical protos, so every
// per-type version can be unchanged while missingEndpointsReferenced shrinks.
// Equals must compare the gap classification itself, or the event is
// suppressed and syncXds never releases a held route flip whose blocking
// cluster derived its (empty) truth.
func TestXdsSnapWrapperEquals_ComparesGapClassification(t *testing.T) {
	newSnap := func() *envoycache.Snapshot {
		snap := &envoycache.Snapshot{}
		snap.Resources[envoycachetypes.Listener] = envoycache.NewResources("l1", nil)
		snap.Resources[envoycachetypes.Route] = envoycache.NewResources("r1", nil)
		snap.Resources[envoycachetypes.Cluster] = envoycache.NewResources("c1", nil)
		snap.Resources[envoycachetypes.Endpoint] = envoycache.NewResources("e1", nil)
		return snap
	}

	deferredWrap := XdsSnapWrapper{
		snap:                       newSnap(),
		proxyKey:                   "client",
		deferred:                   true,
		missingEndpointsReferenced: []string{"cluster-underived"},
	}
	readyWrap := XdsSnapWrapper{
		snap:     newSnap(),
		proxyKey: "client",
	}

	assert.False(t, deferredWrap.Equals(readyWrap),
		"identical versions with different gap classification must not be equal")
	assert.False(t, readyWrap.Equals(deferredWrap),
		"identical versions with different gap classification must not be equal (symmetric)")

	deferredMissing := deferredWrap
	deferredMissing.missingEndpointsReferenced = nil
	deferredMissing.missingReferenced = []string{"cluster-missing"}
	assert.False(t, deferredWrap.Equals(deferredMissing),
		"a gap moving between classifications must not be equal")

	same := deferredWrap
	same.snap = newSnap()
	assert.True(t, deferredWrap.Equals(same),
		"identical versions and identical classification are equal")
}
