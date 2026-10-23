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
				for i := 0; i < 2; i++ {
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
