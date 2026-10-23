package proxy_syncer

import (
	"context"
	"testing"
	"time"

	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoyresourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/stretchr/testify/require"
)

func TestTunnelProxyHoldSurvivesRepeatedBuilds(t *testing.T) {
	for _, scoped := range []bool{false, true} {
		for _, budget := range []time.Duration{0, time.Hour} {
			name := "all"
			if scoped {
				name = "referenced"
			}
			t.Run(name+"/"+budget.String(), func(t *testing.T) {
				pt := newPublishGateTestTranslator(t, false, budget)
				ctx := context.Background()
				pt.syncXds(ctx, routedBuild(t, "v1", "old", "old"))
				heldRoutes := servedSnapshot(t, pt).GetVersion(envoyresourcev3.RouteType)
				wrap := tunnelBuild{version: "v2", withTunnel: true, withProxy: true, secretValue: "s"}.wrapper(t)
				wrap.scopesClusters = scoped
				wrap.missingEndpointsReferenced = []string{"proxy"}
				wrap.synthesizedEndpoints = []string{"proxy"}
				wrap.snap.Resources[envoycachetypes.Endpoint].Items["proxy"] = envoycachetypes.ResourceWithTTL{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "proxy"}}
				for i := range 2 {
					pt.syncXds(ctx, wrap)
					served := servedSnapshot(t, pt)
					require.Equal(t, heldRoutes, served.GetVersion(envoyresourcev3.RouteType), "build %d must keep the route held", i)
					require.True(t, hasResource(served.Resources[envoycachetypes.Listener].Items, "tunnel_x"))
				}
				if budget > 0 {
					pt.gate.mu.Lock()
					pending := pt.gate.pendingFlips[publishGateTestClient]
					if pending != nil && pending.timer != nil {
						pending.timer.Stop() // invoke its callback explicitly below
					}
					pt.gate.mu.Unlock()
					require.NotNil(t, pending)
					pt.gate.fireFlipRelease(ctx, pt.xdsCache, publishGateTestClient, pending)
					require.Equal(t, "rds-v2", servedSnapshot(t, pt).GetVersion(envoyresourcev3.RouteType), "budget expiry releases the route")
					pt.syncXds(ctx, wrap)
					require.Equal(t, "rds-v2", servedSnapshot(t, pt).GetVersion(envoyresourcev3.RouteType), "a released route must not be held again")
				}
				ready := tunnelBuild{version: "v3", withTunnel: true, withProxy: true, secretValue: "s"}.wrapper(t)
				ready.scopesClusters = scoped
				pt.syncXds(ctx, ready)
				require.Equal(t, "rds-v3", servedSnapshot(t, pt).GetVersion(envoyresourcev3.RouteType))
			})
		}
	}
}

func TestTunnelProxyHoldSurvivesConnectedRowWithdrawal(t *testing.T) {
	for _, scoped := range []bool{false, true} {
		for _, budget := range []time.Duration{0, time.Hour} {
			name := "all"
			if scoped {
				name = "referenced"
			}
			t.Run(name+"/"+budget.String(), func(t *testing.T) {
				pt := newPublishGateTestTranslator(t, false, budget)
				ctx := context.Background()
				pt.syncXds(ctx, routedBuild(t, "v1", "old", "old"))
				heldRoutes := servedSnapshot(t, pt).GetVersion(envoyresourcev3.RouteType)
				wrap := tunnelBuild{version: "v2", withTunnel: true, withProxy: true, secretValue: "s"}.wrapper(t)
				wrap.scopesClusters = scoped
				wrap.missingEndpointsReferenced = []string{"proxy"}
				wrap.synthesizedEndpoints = []string{"proxy"}
				wrap.snap.Resources[envoycachetypes.Endpoint].Items["proxy"] = envoycachetypes.ResourceWithTTL{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "proxy"}}
				pt.syncXds(ctx, wrap)
				require.Equal(t, heldRoutes, servedSnapshot(t, pt).GetVersion(envoyresourcev3.RouteType))

				pt.gate.mu.Lock()
				before := *pt.gate.pendingFlips[publishGateTestClient]
				pt.gate.mu.Unlock()
				pt.clientSnapshotDeleted(publishGateTestClient, true)
				pt.syncXds(ctx, wrap)
				require.Equal(t, heldRoutes, servedSnapshot(t, pt).GetVersion(envoyresourcev3.RouteType), "returning row must still wait for proxy endpoints")

				pt.gate.mu.Lock()
				after := *pt.gate.pendingFlips[publishGateTestClient]
				pt.gate.mu.Unlock()
				require.Equal(t, before.unreadySince, after.unreadySince, "withdrawal must not restart the hold budget")
				require.Equal(t, before.deadline, after.deadline)
				require.Greater(t, after.epoch, before.epoch, "callbacks from before withdrawal must be invalidated")
				if budget == 0 {
					require.Nil(t, after.timer)
				} else {
					require.NotNil(t, after.timer, "returning row must rearm the original deadline")
				}

				pt.clientSnapshotDeleted(publishGateTestClient, true)
				pt.gate.mu.Lock()
				suspended := *pt.gate.pendingFlips[publishGateTestClient]
				pt.gate.mu.Unlock()
				require.Nil(t, suspended.timer, "no timer may publish a withdrawn row")
				require.Greater(t, suspended.epoch, after.epoch)
				pt.clientSnapshotDeleted(publishGateTestClient, false)
				pt.gate.mu.Lock()
				defer pt.gate.mu.Unlock()
				require.Empty(t, pt.gate.pendingFlips, "connection departure must discard the hold")
			})
		}
	}
}

// A returning row whose missing-resource blockers have resolved starts a new
// hold episode, so the old budget cannot cap a new reference-ahead window.
func TestReturningRowGetsAFullReferenceAheadWindow(t *testing.T) {
	pt := NewProxyTranslator(newTestSnapshotCache(t), stubPriorXDS{has: true}, time.Hour, true, scopedClustersWithReferenceAhead(time.Hour))
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()
	pt.syncXds(ctx, routedBuild(t, "v1", "old", "old"))
	blocked := routedBuild(t, "v2", "new", "old")
	blocked.missingReferenced = []string{"new"}
	pt.syncXds(ctx, blocked)
	require.Equal(t, "v1", publishedListenerVersionOf(t, &pt), "the flip is held for the unready cluster")

	// Stand in for a withdrawal that outlasts the budget.
	pt.gate.mu.Lock()
	held := pt.gate.pendingFlips[publishGateTestClient]
	if held != nil {
		held.since = held.since.Add(-2 * time.Hour)
	}
	pt.gate.mu.Unlock()
	require.NotNil(t, held)
	pt.clientSnapshotDeleted(publishGateTestClient, true)

	pt.syncXds(ctx, routedBuild(t, "v3", "y", "old", "y"))
	pt.gate.mu.Lock()
	returned := pt.gate.pendingFlips[publishGateTestClient]
	var capped bool
	if returned != nil {
		capped = returned.capped
	}
	pt.gate.mu.Unlock()
	require.NotNil(t, returned, "the routes naming new cluster y wait for its window")
	require.False(t, capped, "the withdrawn hold's start must not cap the new window")
	require.Equal(t, "v1", publishedListenerVersionOf(t, &pt))
}

func TestReturningRowPreservesSurvivingBlockerBudgetCap(t *testing.T) {
	for name, addCluster := range map[string]bool{"same clusters": false, "new reference ahead windows": true} {
		t.Run(name, func(t *testing.T) {
			pt := NewProxyTranslator(newTestSnapshotCache(t), stubPriorXDS{has: true}, time.Hour, true, scopedClustersWithReferenceAhead(time.Hour))
			t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
			ctx := context.Background()
			pt.syncXds(ctx, routedBuild(t, "v1", "old", "old"))
			pt.syncXds(ctx, routedBuild(t, "v2", "new", "old", "new"))
			blocked := routedBuild(t, "v3", "blocked", "old", "new")
			blocked.missingReferenced = []string{"blocked"}
			pt.syncXds(ctx, blocked)

			// The delivery hold began before the missing-resource hold. Advance
			// their ages without sleeping and let the original delivery window close.
			now := time.Now()
			pt.gate.mu.Lock()
			pending := pt.gate.pendingFlips[publishGateTestClient]
			pending.since = now.Add(-20 * time.Minute)
			pending.unreadySince = now.Add(-10 * time.Minute)
			for cluster := range pt.gate.aheadSent[publishGateTestClient] {
				pt.gate.aheadSent[publishGateTestClient][cluster] = now.Add(-2 * time.Hour)
			}
			pt.gate.mu.Unlock()
			pt.syncXds(ctx, blocked)
			pt.gate.mu.Lock()
			before := *pt.gate.pendingFlips[publishGateTestClient]
			pt.gate.mu.Unlock()
			require.Equal(t, before.since.Add(time.Hour), before.deadline)
			require.True(t, before.capped)

			clusters := []string{"old", "new"}
			for _, cluster := range []string{"y", "z"} {
				pt.clientSnapshotDeleted(publishGateTestClient, true)
				if addCluster {
					clusters = append(clusters, cluster)
				}
				returning := routedBuild(t, "return-"+cluster, "blocked", clusters...)
				returning.missingReferenced = []string{"blocked"}
				pt.syncXds(ctx, returning)
				pt.gate.mu.Lock()
				after := *pt.gate.pendingFlips[publishGateTestClient]
				pt.gate.mu.Unlock()
				require.Equal(t, before.deadline, after.deadline, "withdrawal must not extend a surviving blocker's budget cap")
				require.Equal(t, before.since, after.since)
				require.Equal(t, before.unreadySince, after.unreadySince)
				if addCluster {
					require.Contains(t, after.ahead, cluster)
				} else {
					require.Empty(t, after.ahead)
				}
				require.Equal(t, "v1", publishedListenerVersionOf(t, &pt))
			}
		})
	}
}
