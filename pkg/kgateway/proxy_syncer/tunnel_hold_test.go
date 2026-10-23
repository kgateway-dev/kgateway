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
				require.Equal(t, before.since, after.since, "withdrawal must not restart the hold budget")
				require.Equal(t, before.unreadySince, after.unreadySince)
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
