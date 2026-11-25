package proxy_syncer

import (
	"context"
	"testing"
	"time"

	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/validate"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

// defaultsBootstrap is the bootstrap of a pod without the validation annotation.
func defaultsBootstrap() *ir.PodBootstrap {
	return &ir.PodBootstrap{Ports: validate.DefaultReservedPorts()}
}

func TestBootstrapCoverage(t *testing.T) {
	tests := map[string]struct {
		vb   *ir.GatewayValidationBootstraps
		pod  *ir.PodBootstrap
		want bool
	}{
		"a Gateway without overlays covers a pod without the annotation": {
			pod:  defaultsBootstrap(),
			want: true,
		},
		"a Gateway without overlays does not cover a merged subset": {
			pod:  &ir.PodBootstrap{Subset: "s2", Ports: validate.DefaultReservedPorts()},
			want: false,
		},
		"a client without a pod is always covered": {
			vb:   &ir.GatewayValidationBootstraps{Subset: ir.ValidationBootstrap{Subset: "s1"}},
			want: true,
		},
		"the current subset is covered": {
			vb:   &ir.GatewayValidationBootstraps{Subset: ir.ValidationBootstrap{Subset: "s2"}},
			pod:  &ir.PodBootstrap{Subset: "s2", Ports: validate.DefaultReservedPorts()},
			want: true,
		},
		"an earlier subset is not covered": {
			vb:   &ir.GatewayValidationBootstraps{Subset: ir.ValidationBootstrap{Subset: "s1"}},
			pod:  &ir.PodBootstrap{Subset: "s2", Ports: validate.DefaultReservedPorts()},
			want: false,
		},
		"validation data without a subset covers only Envoy's defaults": {
			vb:   &ir.GatewayValidationBootstraps{ReservedPorts: []uint32{9000}},
			pod:  &ir.PodBootstrap{Ports: []uint32{8082, 9000, 9091, 19000}},
			want: true,
		},
		"a port the translation did not reserve is not covered": {
			vb:   &ir.GatewayValidationBootstraps{Subset: ir.ValidationBootstrap{Subset: "s1"}},
			pod:  &ir.PodBootstrap{Subset: "s1", Ports: []uint32{8082, 8443, 9091, 19000}},
			want: false,
		},
		"a default port the current bootstrap no longer listens on stays covered": {
			vb:   &ir.GatewayValidationBootstraps{Subset: ir.ValidationBootstrap{Subset: "s1"}, ReservedPorts: []uint32{8443}},
			pod:  &ir.PodBootstrap{Subset: "s1", Ports: []uint32{8082, 8443}},
			want: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Check the pod's bootstrap against the translation's coverage.
			got := newBootstrapCoverage(tc.vb).covers(tc.pod)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBootstrapCoverageEquals(t *testing.T) {
	// The same subset and ports give the same coverage.
	a := newBootstrapCoverage(&ir.GatewayValidationBootstraps{Subset: ir.ValidationBootstrap{Subset: "s1"}, ReservedPorts: []uint32{8443}})
	b := newBootstrapCoverage(&ir.GatewayValidationBootstraps{Subset: ir.ValidationBootstrap{Subset: "s1"}, ReservedPorts: []uint32{8443}})
	assert.True(t, a.Equals(b), "the same subset and ports must give equal coverage")

	// A subset or a port changes it.
	c := newBootstrapCoverage(&ir.GatewayValidationBootstraps{Subset: ir.ValidationBootstrap{Subset: "s2"}, ReservedPorts: []uint32{8443}})
	assert.False(t, a.Equals(c), "a different subset changes coverage")
	d := newBootstrapCoverage(&ir.GatewayValidationBootstraps{Subset: ir.ValidationBootstrap{Subset: "s1"}})
	assert.False(t, a.Equals(d), "a dropped port changes coverage")

	// GatewayXdsResources compares it.
	x := GatewayXdsResources{validatedBootstraps: a}
	y := GatewayXdsResources{validatedBootstraps: c}
	assert.False(t, x.Equals(y), "GatewayXdsResources must compare the bootstraps the translation was validated under")
}

func TestXdsSnapWrapperEquals_ComparesBootstrapUnvalidated(t *testing.T) {
	// Two builds that differ only in whether they were validated under the
	// client's bootstrap.
	held := coherentWrapperV("v1")
	held.bootstrapUnvalidated = true
	released := coherentWrapperV("v1")

	// They must differ, or the validated build would never reach the gate.
	assert.False(t, held.Equals(released), "a build validated under the client's bootstrap must not equal one that was not")
	assert.False(t, released.Equals(held), "a build validated under the client's bootstrap must not equal one that was not (symmetric)")
}

// unvalidatedWrapperV is coherentWrapperV for a build that was not validated
// under the client's bootstrap.
func unvalidatedWrapperV(version string) XdsSnapWrapper {
	wrap := coherentWrapperV(version)
	wrap.bootstrapUnvalidated = true
	return wrap
}

// A cold client's first build is held, past the first-publish budget, until a
// build validated under its bootstrap arrives.
func TestBootstrapHold_ColdClientHeldUntilValidated(t *testing.T) {
	// Sync a build that was not validated under the client's bootstrap.
	pt := newPublishGateTestTranslator(t, false, 30*time.Millisecond)
	pt.syncXds(context.Background(), unvalidatedWrapperV("v1"))

	// Nothing is published, even after the budget.
	require.Never(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, 150*time.Millisecond, 10*time.Millisecond, "a build not validated under a new client's bootstrap must not be published")

	// A validated build is published at once.
	pt.syncXds(context.Background(), coherentWrapperV("v2"))
	v, ok := publishedListenerVersion(t, pt)
	require.True(t, ok, "a build validated under the client's bootstrap must be published")
	assert.Equal(t, "v2", v)
	assert.Empty(t, pt.gate.bootstrapHeld, "the publish ends the hold")
}

// A warm client already serves configuration and is never held.
func TestBootstrapHold_WarmClientNeverHeld(t *testing.T) {
	// Sync an unvalidated build for a client that reported a prior xDS version.
	pt := newPublishGateTestTranslator(t, true, 30*time.Millisecond)
	pt.syncXds(context.Background(), unvalidatedWrapperV("v1"))

	// It is published at once.
	v, ok := publishedListenerVersion(t, pt)
	require.True(t, ok, "a warm client must never be held for its bootstrap")
	assert.Equal(t, "v1", v)
}

// stubReadyPodXDS is a cold client whose stream was admitted while its pod was
// Ready, or not.
type stubReadyPodXDS struct {
	stubPriorXDS
	ready bool
}

func (s stubReadyPodXDS) HasReadyPodStream(string) bool { return s.ready }

// A client whose pod was Ready when its stream was admitted already serves
// configuration from an earlier stream, and is never held, even when the Gateway's
// current bootstrap is not the one it started with. A pod that is not Ready is
// still held.
func TestBootstrapHold_ReadyPodNeverHeld(t *testing.T) {
	for name, tc := range map[string]struct {
		ready         bool
		wantPublished bool
	}{
		"a Ready pod is published at once":   {ready: true, wantPublished: true},
		"a pod that is not Ready stays held": {ready: false, wantPublished: false},
	} {
		t.Run(name, func(t *testing.T) {
			// Sync an unvalidated build for a cold client.
			pt := NewProxyTranslator(newTestSnapshotCache(t), stubReadyPodXDS{ready: tc.ready}, 30*time.Millisecond, true, clusterScoping{})
			t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
			pt.syncXds(context.Background(), unvalidatedWrapperV("v1"))

			// Check whether it was published.
			if tc.wantPublished {
				v, ok := publishedListenerVersion(t, &pt)
				require.True(t, ok, "a Ready pod must never be held for its bootstrap")
				assert.Equal(t, "v1", v)
				return
			}
			require.Never(t, func() bool {
				_, ok := publishedListenerVersion(t, &pt)
				return ok
			}, 150*time.Millisecond, 10*time.Millisecond, "a pod that is not Ready must be held for its bootstrap")
		})
	}
}

// A client published on its connection is never held again.
func TestBootstrapHold_PublishedClientNotHeld(t *testing.T) {
	// Publish a validated build first.
	pt := newPublishGateTestTranslator(t, false, 30*time.Millisecond)
	pt.syncXds(context.Background(), coherentWrapperV("v1"))

	// A later unvalidated build is still published.
	pt.syncXds(context.Background(), unvalidatedWrapperV("v2"))
	v, ok := publishedListenerVersion(t, pt)
	require.True(t, ok)
	assert.Equal(t, "v2", v, "a client that already has configuration must not be held")
}

// Holding drops a stored deferred build, so its first-publish timer cannot
// publish a build older than the held one.
func TestBootstrapHold_DropsStoredFirstPublish(t *testing.T) {
	// Store a deferred validated build, then hold an unvalidated one.
	pt := newPublishGateTestTranslator(t, false, 30*time.Millisecond)
	pt.syncXds(context.Background(), deferredWrapperV("v1"))
	pt.syncXds(context.Background(), unvalidatedWrapperV("v2"))

	// Neither is published at the budget.
	require.Never(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, 150*time.Millisecond, 10*time.Millisecond, "the stored first-publish build must not be published while a newer build is held")

	// A validated deferred build gets the budget that remains, which is spent.
	pt.syncXds(context.Background(), deferredWrapperV("v3"))
	require.Eventually(t, func() bool {
		v, ok := publishedListenerVersion(t, pt)
		return ok && v == "v3"
	}, time.Second, 5*time.Millisecond, "a validated deferred build must publish once the remaining budget passes")
}

// A departure ends the hold's bookkeeping.
func TestBootstrapHold_DepartureForgetsHold(t *testing.T) {
	// Hold a build, then let the client leave.
	pt := newPublishGateTestTranslator(t, false, 0)
	pt.syncXds(context.Background(), unvalidatedWrapperV("v1"))
	require.Contains(t, pt.gate.bootstrapHeld, publishGateTestClient)
	pt.gate.clientDeparted(publishGateTestClient)

	// Nothing about the hold is kept.
	assert.NotContains(t, pt.gate.bootstrapHeld, publishGateTestClient)
}

// bootstrapHoldPipeline runs a client with bootstrap through snapshotPerClient and
// the gate, with the Gateway translated under coverage, and returns the cache and
// the Gateway's mutable translation output.
func bootstrapHoldPipeline(t *testing.T, bootstrap *ir.PodBootstrap, coverage bootstrapCoverage) (envoycache.SnapshotCache, string, krt.StaticCollection[GatewayXdsResources]) {
	t.Helper()

	// Build a client of the Gateway with the pod's bootstrap.
	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "pod-ns", map[string]string{"pod-template-hash": "abc"}, ir.PodLocality{})
	ucc.Bootstrap = bootstrap
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})

	// Translate the Gateway with routes to one cluster that has its endpoints.
	routes := routeResourcesForClusters("cluster-a")
	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	mostXdsSnapshots := krt.NewMutableCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
		NamespacedName:      types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:              routes,
		Listeners:           listeners,
		ReferencedClusters:  collectReferencedClusters(routes, listeners),
		validatedBootstraps: coverage,
	}})
	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{edsClusterForClient(ucc, "cluster-a", 1)})
	endpointCol := krt.NewStaticCollection[UccWithEndpoints](nil, []UccWithEndpoints{endpointsForClient(ucc, "cluster-a", 3)})

	// Publish every row through a cold client's gate.
	snapshots := snapshotPerClient(
		krtutil.KrtOptions{},
		uccs,
		mostXdsSnapshots.AsCollection(),
		PerClientEnvoyEndpoints{
			endpoints: endpointCol,
			index: krtpkg.UnnamedIndex(endpointCol, func(ep UccWithEndpoints) []string {
				return []string{ep.Client.ResourceName()}
			}),
		},
		newTestPerClientClustersFromCol(clusterCol, uccs),
		clusterScoping{},
	)
	cache := newTestSnapshotCache(t)
	translator := NewProxyTranslator(cache, stubPriorXDS{has: false}, 0, true, clusterScoping{})
	t.Cleanup(func() { translator.gate.clientDeparted(ucc.ResourceName()) })
	registerSyncXds(snapshots, translator)
	return cache, ucc.ResourceName(), mostXdsSnapshots
}

// A new pod is held until the Gateway is translated under its bootstrap, then
// receives the same configuration.
func TestBootstrapHold_PipelineReleasesWhenTranslationCoversPod(t *testing.T) {
	// The Gateway is translated under s1; the new pod runs s2.
	s1 := newBootstrapCoverage(&ir.GatewayValidationBootstraps{Subset: ir.ValidationBootstrap{Subset: "s1"}})
	cache, key, gw := bootstrapHoldPipeline(t, &ir.PodBootstrap{Subset: "s2", Ports: validate.DefaultReservedPorts()}, s1)

	// The pod gets nothing.
	require.Never(t, func() bool {
		_, err := cache.GetSnapshot(key)
		return err == nil
	}, 200*time.Millisecond, 20*time.Millisecond, "a pod whose bootstrap the translation was not validated under must get no configuration")

	// Translating the Gateway under s2 releases it, with no other change.
	s2 := newBootstrapCoverage(&ir.GatewayValidationBootstraps{Subset: ir.ValidationBootstrap{Subset: "s2"}})
	current := gw.List()[0]
	current.validatedBootstraps = s2
	gw.UpdateObject(current)
	snap := eventuallyCacheSnapshot(t, cache, key)
	assert.Contains(t, snap.Resources[envoycachetypes.Listener].Items, "listener")
}

// A Gateway without bootstrap overlays publishes to a pod without the annotation at once.
func TestBootstrapHold_PipelineGatewayWithoutOverlaysUnaffected(t *testing.T) {
	// Neither the Gateway nor the pod has bootstrap overlays.
	cache, key, _ := bootstrapHoldPipeline(t, defaultsBootstrap(), newBootstrapCoverage(nil))

	// The pod gets its configuration without a hold.
	snap := eventuallyCacheSnapshot(t, cache, key)
	assert.Contains(t, snap.Resources[envoycachetypes.Listener].Items, "listener")
}
