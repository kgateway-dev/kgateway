package proxy_syncer

import (
	"context"
	"fmt"
	"maps"
	"runtime"
	"strconv"
	"testing"
	"time"

	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
)

const (
	benchRoutes    = 5000
	benchListeners = 50
	benchClusters  = 100
)

// benchRoutingConfig returns a gateway's routes and listeners: benchRoutes
// routes over benchClusters clusters, behind benchListeners HTTP listeners
// whose connection managers a routing scan unpacks. Each call allocates new
// protos, as a new translation does.
func benchRoutingConfig(version string) (envoycache.Resources, envoycache.Resources) {
	routes := make([]*envoyroutev3.Route, 0, benchRoutes)
	for i := range benchRoutes {
		routes = append(routes, &envoyroutev3.Route{
			Name: fmt.Sprintf("route-%s-%d", version, i),
			Match: &envoyroutev3.RouteMatch{
				PathSpecifier: &envoyroutev3.RouteMatch_Prefix{Prefix: fmt.Sprintf("/%s/%d", version, i)},
			},
			Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{
				ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: "cluster-" + strconv.Itoa(i%benchClusters)},
			}},
		})
	}
	routeConfigs := []*envoyroutev3.RouteConfiguration{{
		Name:         "route-config",
		VirtualHosts: []*envoyroutev3.VirtualHost{{Name: "vhost", Domains: []string{"*"}, Routes: routes}},
	}}
	listeners := make([]*envoylistenerv3.Listener, 0, benchListeners)
	for i := range benchListeners {
		hcm := &envoyhcmv3.HttpConnectionManager{
			StatPrefix: fmt.Sprintf("listener-%s-%d", version, i),
			RouteSpecifier: &envoyhcmv3.HttpConnectionManager_Rds{
				Rds: &envoyhcmv3.Rds{RouteConfigName: "route-config"},
			},
		}
		listeners = append(listeners, &envoylistenerv3.Listener{
			Name: fmt.Sprintf("listener-%d", i),
			FilterChains: []*envoylistenerv3.FilterChain{{Filters: []*envoylistenerv3.Filter{{
				Name:       "envoy.filters.network.http_connection_manager",
				ConfigType: &envoylistenerv3.Filter_TypedConfig{TypedConfig: utils.MustMessageToAny(hcm)},
			}}}},
		})
	}
	return sliceToResources(routeConfigs), sliceToResources(listeners)
}

// benchClusterResources returns CDS for every routed cluster, and EDS whose
// version is endpoints.
func benchClusterResources(endpoints uint64) (envoycache.Resources, envoycache.Resources) {
	ucc := newTestUcc(publishGateTestClient)
	var clusters, claItems []envoycachetypes.ResourceWithTTL
	for i := range benchClusters {
		name := "cluster-" + strconv.Itoa(i)
		clusters = append(clusters, edsClusterForClient(ucc, name, uint64(i)).Cluster.ResourceWithTTL())
		claItems = append(claItems, endpointsForClient(ucc, name, endpoints+uint64(i)).Endpoints.ResourceWithTTL())
	}
	return envoycache.NewResourcesWithTTL("cds", clusters),
		envoycache.NewResourcesWithTTL("eds-"+strconv.FormatUint(endpoints, 10), claItems)
}

// benchBuild is a client build over routes and listeners. routing is their
// routing targets, computed once per configuration as snapshotPerClient
// composes them. missing marks a referenced cluster's CLA underived, so the
// gate resolves the build against the published snapshot.
func benchBuild(proxyKey string, routes, listeners, clusters, endpoints envoycache.Resources, routing routingTargets, missing ...string) XdsSnapWrapper {
	snap := &envoycache.Snapshot{}
	snap.Resources[envoycachetypes.Route] = routes
	snap.Resources[envoycachetypes.Listener] = listeners
	snap.Resources[envoycachetypes.Cluster] = clusters
	snap.Resources[envoycachetypes.Endpoint] = endpoints
	return XdsSnapWrapper{snap: snap, routing: routing, proxyKey: proxyKey, missingEndpointsReferenced: missing}
}

func newBenchTranslator() ProxyTranslator {
	// No consistency check: it scans every publish.
	return NewProxyTranslator(envoycache.NewSnapshotCache(true, envoycache.IDHash{}, nil), stubPriorXDS{}, time.Hour, false, clusterScoping{})
}

// BenchmarkDeferredEndpointUpdate times endpoint-only deferred builds of one
// large routing configuration; the gate lock is held for all of each one.
// shared-listeners reuses the translation's listener map, as a gateway without
// backend listeners does; merged-listeners copies it per build, as merging a
// backend's own listeners (a CONNECT tunnel) does.
func BenchmarkDeferredEndpointUpdate(b *testing.B) {
	for _, merged := range []bool{false, true} {
		name := "shared-listeners"
		if merged {
			name = "merged-listeners"
		}
		b.Run(name, func(b *testing.B) {
			ctx := context.Background()
			pt := newBenchTranslator()
			routes, listeners := benchRoutingConfig("v1")
			routing := newRoutingTargets(collectReferencedClusters(routes, listeners))
			clusters, endpoints := benchClusterResources(0)
			pt.syncXds(ctx, benchBuild(publishGateTestClient, routes, listeners, clusters, endpoints, routing))
			endpointSets := make([]envoycache.Resources, 2)
			for i := range endpointSets {
				_, endpointSets[i] = benchClusterResources(uint64(1000 * (i + 1)))
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				buildListeners := listeners
				if merged {
					buildListeners = envoycache.Resources{Version: listeners.Version, Items: maps.Clone(listeners.Items)}
				}
				pt.syncXds(ctx, benchBuild(publishGateTestClient, routes, buildListeners, clusters, endpointSets[i%2], routing, "cluster-0"))
			}
		})
	}
}

// BenchmarkRetainedAfterRoutingChurn gives each of gateways clients its own
// large routing configuration and replaces it churn times through deferred
// builds, each resolved against the last. It then deletes every route (an
// empty publish) and reports the heap still allocated after a GC, relative to
// before: with the clients still connected (their gateways remain), and after
// they depart.
func BenchmarkRetainedAfterRoutingChurn(b *testing.B) {
	for _, tc := range []struct {
		name            string
		gateways, churn int
		depart          bool
	}{
		{name: "1-gateway-20-builds/connected", gateways: 1, churn: 20},
		{name: "1-gateway-20-builds/departed", gateways: 1, churn: 20, depart: true},
		{name: "8-gateways-3-builds/connected", gateways: 8, churn: 3},
		{name: "8-gateways-3-builds/departed", gateways: 8, churn: 3, depart: true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			ctx := context.Background()
			for range b.N {
				runtime.GC()
				var before runtime.MemStats
				runtime.ReadMemStats(&before)

				pt := newBenchTranslator()
				clusters, endpoints := benchClusterResources(0)
				empty := envoycache.Resources{Version: "empty"}
				for g := range tc.gateways {
					proxyKey := "client-" + strconv.Itoa(g)
					for i := range tc.churn {
						routes, listeners := benchRoutingConfig(fmt.Sprintf("g%d-v%d", g, i))
						routing := newRoutingTargets(collectReferencedClusters(routes, listeners))
						var missing []string
						if i > 0 {
							missing = []string{"cluster-0"}
						}
						pt.syncXds(ctx, benchBuild(proxyKey, routes, listeners, clusters, endpoints, routing, missing...))
					}
					pt.syncXds(ctx, benchBuild(proxyKey, empty, empty, empty, empty, routingTargets{}))
					if tc.depart {
						pt.gate.clientDeparted(proxyKey)
					}
				}

				runtime.GC()
				var after runtime.MemStats
				runtime.ReadMemStats(&after)
				b.ReportMetric((float64(after.HeapAlloc)-float64(before.HeapAlloc))/(1<<20), "retained-MiB")
				runtime.KeepAlive(pt)
			}
		})
	}
}
