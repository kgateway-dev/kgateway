package setup

import (
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	cachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/status"
)

// These tests drive our ADS cache through the go-control-plane SotW server with
// the requests Envoy sends when it rejects one resource of a CDS or LDS
// response, and require that the proxy still gets the endpoint and route
// updates it subscribed to.
//
// Envoy applies the valid resources of a NACKed CDS or LDS response and never
// subscribes to the rejected resource's endpoints or routes. In ADS mode the
// pinned go-control-plane answers a named EDS or RDS request only when the
// snapshot holds nothing outside the request. On its plain ADS cache:
//
//   - while the rejected resource is in the snapshot, its CLA or
//     RouteConfiguration holds back every endpoint or route update for the
//     proxy, not only its own;
//   - the request Envoy sends for a valid resource added in the same response
//     is declined on the request path, which registers no watch. Removing the
//     rejected resource then has nothing to answer, and the proxy stays on its
//     old endpoints or routes until its subscription changes again or it
//     reconnects;
//   - a proxy that connects while the rejected resource is published loses its
//     first EDS or RDS request the same way. Its cluster initializes empty at
//     the fetch timeout, or its listener with no routes, so it reports ready
//     while the route fails.
//
// The request sequences are the ones Envoy v1.39.4 sent to this pinned
// go-control-plane with ordered ADS, with an EDS cluster rejected for
// CLUSTER_PROVIDED load balancing or an unrecognized TLS signature algorithm,
// and a listener rejected for an HTTP filter with no registered factory. In
// those runs the added cluster started empty at its 15s EDS fetch timeout, the
// added listener started with no routes at its RDS fetch timeout, and the
// existing cluster and route kept their first version through two later
// updates and the rejected resource's removal.
//
// newSnapshotCache serves EDS, RDS and SDS (and LDS, to keep ADS's order) from
// non-ADS caches that answer the requested names (perTypeCache), so these tests
// pass. On the plain ADS cache they all fail. A go-control-plane that answers
// named ADS requests with only the subscribed resources would pass them all on
// its own; one that keeps the watch of a request it declines would pass only
// the two "after it is removed" tests.

const proxyUpdateWithin = 2 * time.Second

func adsEDSCluster(name string) *envoyclusterv3.Cluster {
	return &envoyclusterv3.Cluster{
		Name:                 name,
		ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS},
		EdsClusterConfig: &envoyclusterv3.Cluster_EdsClusterConfig{
			EdsConfig: &envoycorev3.ConfigSource{
				ResourceApiVersion:    envoycorev3.ApiVersion_V3,
				ConfigSourceSpecifier: &envoycorev3.ConfigSource_Ads{Ads: &envoycorev3.AggregatedConfigSource{}},
			},
		},
	}
}

func adsEndpoints(cluster, address string) *envoyendpointv3.ClusterLoadAssignment {
	return &envoyendpointv3.ClusterLoadAssignment{
		ClusterName: cluster,
		Endpoints: []*envoyendpointv3.LocalityLbEndpoints{{
			LbEndpoints: []*envoyendpointv3.LbEndpoint{{
				HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{Endpoint: &envoyendpointv3.Endpoint{
					Address: &envoycorev3.Address{Address: &envoycorev3.Address_SocketAddress{SocketAddress: &envoycorev3.SocketAddress{
						Address:       address,
						PortSpecifier: &envoycorev3.SocketAddress_PortValue{PortValue: 8080},
					}}},
				}},
			}},
		}},
	}
}

// clusterEndpointSnapshot versions CDS and EDS separately, as kgateway does: a
// change to one type leaves the other's version alone. endpoints maps each
// cluster to its one endpoint address.
func clusterEndpointSnapshot(cdsVersion, edsVersion string, clusters []string, endpoints map[string]string) *envoycache.Snapshot {
	clusterItems := make([]cachetypes.Resource, 0, len(clusters))
	for _, name := range clusters {
		clusterItems = append(clusterItems, adsEDSCluster(name))
	}
	endpointItems := make([]cachetypes.Resource, 0, len(endpoints))
	for name, address := range endpoints {
		endpointItems = append(endpointItems, adsEndpoints(name, address))
	}
	snap := &envoycache.Snapshot{}
	snap.Resources[cachetypes.Cluster] = envoycache.NewResources(cdsVersion, clusterItems)
	snap.Resources[cachetypes.Endpoint] = envoycache.NewResources(edsVersion, endpointItems)
	return snap
}

// listenerRouteSnapshot is clusterEndpointSnapshot for LDS and RDS. routes maps
// each RouteConfiguration to the cluster its one route targets, which the
// tests use as the route's content.
func listenerRouteSnapshot(ldsVersion, rdsVersion string, listeners []string, routes map[string]string) *envoycache.Snapshot {
	listenerItems := make([]cachetypes.Resource, 0, len(listeners))
	for _, name := range listeners {
		listenerItems = append(listenerItems, &envoylistenerv3.Listener{Name: name})
	}
	routeItems := make([]cachetypes.Resource, 0, len(routes))
	for name, cluster := range routes {
		routeItems = append(routeItems, testRouteConfig(name, cluster))
	}
	snap := &envoycache.Snapshot{}
	snap.Resources[cachetypes.Listener] = envoycache.NewResources(ldsVersion, listenerItems)
	snap.Resources[cachetypes.Route] = envoycache.NewResources(rdsVersion, routeItems)
	return snap
}

// adsProxy plays Envoy's side of the stream. It ACKs every EDS and RDS response
// with its current subscription, as Envoy does, and keeps the content it last
// received for each name. CDS and LDS responses are left to the test: Envoy
// acknowledges CDS only once its new clusters have warmed, and the test decides
// when Envoy NACKs.
type adsProxy struct {
	h *adsHarness
	// names is the subscription per named type.
	names map[string][]string
	// version is the version Envoy holds per type; nonce is that of the last
	// response it received. Envoy sends both on every request.
	version map[string]string
	nonce   map[string]string
	// received maps each named type to resource name -> content: an endpoint
	// address for EDS, a route's target cluster for RDS.
	received map[string]map[string]string
	pending  []*discoveryv3.DiscoveryResponse
}

func newADSProxy(h *adsHarness) *adsProxy {
	return &adsProxy{
		h:        h,
		names:    map[string][]string{},
		version:  map[string]string{},
		nonce:    map[string]string{},
		received: map[string]map[string]string{envoyresource.EndpointType: {}, envoyresource.RouteType: {}},
	}
}

func (p *adsProxy) request(typeURL string, errorDetail *status.Status) {
	p.h.t.Helper()

	p.h.send(&discoveryv3.DiscoveryRequest{
		Node:          p.h.node,
		TypeUrl:       typeURL,
		ResourceNames: p.names[typeURL],
		VersionInfo:   p.version[typeURL],
		ResponseNonce: p.nonce[typeURL],
		ErrorDetail:   errorDetail,
	})
}

// subscribe requests names, at the version Envoy holds and the nonce of the
// last response it received.
func (p *adsProxy) subscribe(typeURL string, names ...string) {
	p.h.t.Helper()

	p.names[typeURL] = names
	p.request(typeURL, nil)
}

// subscribeHandled is subscribe, then waits until the cache has handled a
// request carrying the new names, whatever it did with it. CreateWatch stamps
// the node's last watch request time while holding the cache lock that
// SetSnapshot takes, so a snapshot set afterwards is ordered after the request,
// as it is with Envoy, which sends it milliseconds after the CDS or LDS
// response. If a response raced the request, the server drops the request for
// its stale nonce; handling that response sends the names again, as Envoy's
// ACK does.
func (p *adsProxy) subscribeHandled(typeURL string, names ...string) {
	p.h.t.Helper()

	sent := time.Now()
	p.subscribe(typeURL, names...)
	handled := p.pump(proxyUpdateWithin, func() bool {
		info := p.h.cache.GetStatusInfo(adsTestNodeID)
		return info != nil && !info.GetLastWatchRequestTime().Before(sent)
	})
	require.True(p.h.t, handled, "the cache never handled the %s request", typeURL)
}

func (p *adsProxy) accept(resp *discoveryv3.DiscoveryResponse) {
	p.h.t.Helper()

	p.version[resp.GetTypeUrl()] = resp.GetVersionInfo()
	p.request(resp.GetTypeUrl(), nil)
}

// reject NACKs resp: the request keeps the version Envoy holds.
func (p *adsProxy) reject(resp *discoveryv3.DiscoveryResponse) {
	p.h.t.Helper()

	p.request(resp.GetTypeUrl(), &status.Status{Message: "rejected by test"})
}

func (p *adsProxy) handle(resp *discoveryv3.DiscoveryResponse) {
	p.h.t.Helper()

	typeURL := resp.GetTypeUrl()
	p.nonce[typeURL] = resp.GetNonce()
	got, named := p.received[typeURL]
	if !named {
		p.pending = append(p.pending, resp)
		return
	}
	for _, r := range resp.GetResources() {
		switch typeURL {
		case envoyresource.EndpointType:
			cla := &envoyendpointv3.ClusterLoadAssignment{}
			require.NoError(p.h.t, r.UnmarshalTo(cla))
			got[cla.GetClusterName()] = cla.GetEndpoints()[0].GetLbEndpoints()[0].GetEndpoint().GetAddress().GetSocketAddress().GetAddress()
		case envoyresource.RouteType:
			rc := &envoyroutev3.RouteConfiguration{}
			require.NoError(p.h.t, r.UnmarshalTo(rc))
			got[rc.GetName()] = rc.GetVirtualHosts()[0].GetRoutes()[0].GetRoute().GetCluster()
		}
	}
	p.accept(resp)
}

// pump handles responses until done reports true or within passes. done is
// rechecked on every response and every few milliseconds, because some
// conditions change without a response.
func (p *adsProxy) pump(within time.Duration, done func() bool) bool {
	p.h.t.Helper()

	deadline := time.After(within)
	poll := time.NewTicker(5 * time.Millisecond)
	defer poll.Stop()
	for !done() {
		select {
		case resp := <-p.h.stream.sent:
			p.handle(resp)
		case err := <-p.h.done:
			require.NoError(p.h.t, err)
			return false
		case <-poll.C:
		case <-deadline:
			return false
		}
	}
	return true
}

// next returns the next CDS or LDS response, handling named responses meanwhile.
func (p *adsProxy) next(typeURL string) *discoveryv3.DiscoveryResponse {
	p.h.t.Helper()

	var resp *discoveryv3.DiscoveryResponse
	found := p.pump(proxyUpdateWithin, func() bool {
		for i, r := range p.pending {
			if r.GetTypeUrl() == typeURL {
				resp = r
				p.pending = append(p.pending[:i], p.pending[i+1:]...)
				return true
			}
		}
		return false
	})
	require.True(p.h.t, found, "no %s response", typeURL)
	return resp
}

// await reports whether the proxy receives want for typeURL within the update window.
func (p *adsProxy) await(typeURL string, want map[string]string) bool {
	p.h.t.Helper()

	return p.pump(proxyUpdateWithin, func() bool {
		for name, content := range want {
			if p.received[typeURL][name] != content {
				return false
			}
		}
		return true
	})
}

// startWithBackend brings the proxy to one accepted EDS cluster, backend at
// 10.0.0.1, with its CDS and EDS watches parked.
func startWithBackend(t *testing.T, p *adsProxy) {
	t.Helper()

	p.h.setSnapshot(clusterEndpointSnapshot("1", "1", []string{"backend"}, map[string]string{"backend": "10.0.0.1"}))
	p.subscribe(envoyresource.ClusterType)
	p.accept(p.next(envoyresource.ClusterType))
	p.subscribe(envoyresource.EndpointType, "backend")
	require.True(t, p.await(envoyresource.EndpointType, map[string]string{"backend": "10.0.0.1"}), "initial endpoints")
	p.h.waitForOpenWatches(2)
}

// addRejectedCluster publishes cluster "added" and cluster "rejected", which
// Envoy rejects, and sends what Envoy sends next: an EDS request for the
// clusters it applied. Envoy acknowledges the CDS response only after "added"
// warms. It returns the CDS response Envoy will NACK.
func addRejectedCluster(p *adsProxy) *discoveryv3.DiscoveryResponse {
	p.h.t.Helper()

	p.h.setSnapshot(clusterEndpointSnapshot("2", "2",
		[]string{"backend", "added", "rejected"},
		map[string]string{"backend": "10.0.0.1", "added": "10.0.0.2", "rejected": "10.0.0.3"}))
	cds := p.next(envoyresource.ClusterType)
	p.subscribeHandled(envoyresource.EndpointType, "added", "backend")
	return cds
}

// startWithFront is startWithBackend for one listener, front, whose routes
// target backend-v1.
func startWithFront(t *testing.T, p *adsProxy) {
	t.Helper()

	p.h.setSnapshot(listenerRouteSnapshot("1", "1", []string{"front"}, map[string]string{"front-routes": "backend-v1"}))
	p.subscribe(envoyresource.ListenerType)
	p.accept(p.next(envoyresource.ListenerType))
	p.subscribe(envoyresource.RouteType, "front-routes")
	require.True(t, p.await(envoyresource.RouteType, map[string]string{"front-routes": "backend-v1"}), "initial routes")
	p.h.waitForOpenWatches(2)
}

// addRejectedListener publishes listener "added" and listener "rejected",
// which Envoy rejects, and sends the RDS request for the listeners it applied.
// It returns the LDS response Envoy NACKs.
func addRejectedListener(p *adsProxy) *discoveryv3.DiscoveryResponse {
	p.h.t.Helper()

	p.h.setSnapshot(listenerRouteSnapshot("2", "2",
		[]string{"front", "added", "rejected"},
		map[string]string{"front-routes": "backend-v1", "added-routes": "backend-v1", "rejected-routes": "backend-v1"}))
	lds := p.next(envoyresource.ListenerType)
	p.subscribeHandled(envoyresource.RouteType, "added-routes", "front-routes")
	return lds
}

// A cluster the proxy rejected must not hold back the endpoints of the
// clusters it accepted.
func TestADSCacheDeliversEndpointsWhileAClusterIsRejected(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(adsModeName(ordered), func(t *testing.T) {
			p := newADSProxy(newADSHarness(t, ordered))
			startWithBackend(t, p)
			addRejectedCluster(p)

			p.h.setSnapshot(clusterEndpointSnapshot("2", "3",
				[]string{"backend", "added", "rejected"},
				map[string]string{"backend": "10.0.0.11", "added": "10.0.0.2", "rejected": "10.0.0.3"}))
			want := map[string]string{"backend": "10.0.0.11", "added": "10.0.0.2"}
			require.Truef(t, p.await(envoyresource.EndpointType, want),
				"the proxy has endpoints %v, want %v: go-control-plane withholds every EDS response that carries "+
					"the rejected cluster's endpoints, which the proxy never requested", p.received[envoyresource.EndpointType], want)
		})
	}
}

// Once the rejected cluster is gone, the proxy must get the current endpoints
// without having to change its subscription or reconnect.
func TestADSCacheDeliversEndpointsAfterARejectedClusterIsRemoved(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(adsModeName(ordered), func(t *testing.T) {
			p := newADSProxy(newADSHarness(t, ordered))
			startWithBackend(t, p)
			rejected := addRejectedCluster(p)

			// kgateway drops the rejected cluster and moves backend; Envoy then
			// NACKs the CDS response that carried it and accepts the next one.
			p.h.setSnapshot(clusterEndpointSnapshot("3", "4",
				[]string{"backend", "added"},
				map[string]string{"backend": "10.0.0.11", "added": "10.0.0.2"}))
			p.reject(rejected)
			p.accept(p.next(envoyresource.ClusterType))
			want := map[string]string{"backend": "10.0.0.11", "added": "10.0.0.2"}
			require.Truef(t, p.await(envoyresource.EndpointType, want),
				"the proxy has endpoints %v, want %v: go-control-plane declined the EDS request for the accepted "+
					"clusters without keeping its watch, so nothing answers once the rejected cluster is gone",
				p.received[envoyresource.EndpointType], want)
		})
	}
}

// A listener the proxy rejected must not hold back the routes of the listeners
// it accepted.
func TestADSCacheDeliversRoutesWhileAListenerIsRejected(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(adsModeName(ordered), func(t *testing.T) {
			p := newADSProxy(newADSHarness(t, ordered))
			startWithFront(t, p)
			addRejectedListener(p)

			p.h.setSnapshot(listenerRouteSnapshot("2", "3",
				[]string{"front", "added", "rejected"},
				map[string]string{"front-routes": "backend-v2", "added-routes": "backend-v1", "rejected-routes": "backend-v1"}))
			want := map[string]string{"front-routes": "backend-v2", "added-routes": "backend-v1"}
			require.Truef(t, p.await(envoyresource.RouteType, want),
				"the proxy has routes %v, want %v: go-control-plane withholds every RDS response that carries "+
					"the rejected listener's routes, which the proxy never requested", p.received[envoyresource.RouteType], want)
		})
	}
}

// Once the rejected listener is gone, the proxy must get the current routes
// without having to change its subscription or reconnect.
func TestADSCacheDeliversRoutesAfterARejectedListenerIsRemoved(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(adsModeName(ordered), func(t *testing.T) {
			p := newADSProxy(newADSHarness(t, ordered))
			startWithFront(t, p)
			rejected := addRejectedListener(p)

			// kgateway drops the rejected listener and changes front's routes;
			// Envoy NACKs the LDS response that carried it and accepts the next.
			p.h.setSnapshot(listenerRouteSnapshot("3", "4",
				[]string{"front", "added"},
				map[string]string{"front-routes": "backend-v2", "added-routes": "backend-v1"}))
			p.reject(rejected)
			p.accept(p.next(envoyresource.ListenerType))
			want := map[string]string{"front-routes": "backend-v2", "added-routes": "backend-v1"}
			require.Truef(t, p.await(envoyresource.RouteType, want),
				"the proxy has routes %v, want %v: go-control-plane declined the RDS request for the accepted "+
					"listeners without keeping its watch, so nothing answers once the rejected listener is gone",
				p.received[envoyresource.RouteType], want)
		})
	}
}

// A proxy that connects while a cluster it will reject is already published
// must get the endpoints of the clusters it accepts, and their later changes.
func TestADSCacheDeliversEndpointsToANewProxyWhileAClusterIsRejected(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(adsModeName(ordered), func(t *testing.T) {
			p := newADSProxy(newADSHarness(t, ordered))
			p.h.setSnapshot(clusterEndpointSnapshot("1", "1",
				[]string{"backend", "rejected"},
				map[string]string{"backend": "10.0.0.1", "rejected": "10.0.0.3"}))
			// Envoy applies backend, rejects the other cluster, and requests
			// endpoints for backend. It answers CDS only once backend warms.
			p.subscribe(envoyresource.ClusterType)
			rejected := p.next(envoyresource.ClusterType)
			p.subscribeHandled(envoyresource.EndpointType, "backend")
			want := map[string]string{"backend": "10.0.0.1"}
			require.Truef(t, p.await(envoyresource.EndpointType, want),
				"the new proxy has endpoints %v, want %v: go-control-plane declined its first EDS request without "+
					"keeping a watch, so backend initializes empty and its routes fail", p.received[envoyresource.EndpointType], want)

			p.h.setSnapshot(clusterEndpointSnapshot("2", "2",
				[]string{"backend"},
				map[string]string{"backend": "10.0.0.11"}))
			p.reject(rejected)
			p.accept(p.next(envoyresource.ClusterType))
			want = map[string]string{"backend": "10.0.0.11"}
			require.Truef(t, p.await(envoyresource.EndpointType, want),
				"the new proxy has endpoints %v, want %v after the rejected cluster was removed", p.received[envoyresource.EndpointType], want)
		})
	}
}

// A proxy that connects while a listener it will reject is already published
// must get the routes of the listeners it accepts, and their later changes.
func TestADSCacheDeliversRoutesToANewProxyWhileAListenerIsRejected(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(adsModeName(ordered), func(t *testing.T) {
			p := newADSProxy(newADSHarness(t, ordered))
			p.h.setSnapshot(listenerRouteSnapshot("1", "1",
				[]string{"front", "rejected"},
				map[string]string{"front-routes": "backend-v1", "rejected-routes": "backend-v1"}))
			p.subscribe(envoyresource.ListenerType)
			rejected := p.next(envoyresource.ListenerType)
			p.subscribeHandled(envoyresource.RouteType, "front-routes")
			want := map[string]string{"front-routes": "backend-v1"}
			require.Truef(t, p.await(envoyresource.RouteType, want),
				"the new proxy has routes %v, want %v: go-control-plane declined its first RDS request without "+
					"keeping a watch, so front initializes with no routes", p.received[envoyresource.RouteType], want)

			p.h.setSnapshot(listenerRouteSnapshot("2", "2",
				[]string{"front"},
				map[string]string{"front-routes": "backend-v2"}))
			p.reject(rejected)
			p.accept(p.next(envoyresource.ListenerType))
			want = map[string]string{"front-routes": "backend-v2"}
			require.Truef(t, p.await(envoyresource.RouteType, want),
				"the new proxy has routes %v, want %v after the rejected listener was removed", p.received[envoyresource.RouteType], want)
		})
	}
}

// SetSnapshot installs one type at a time, so Envoy can request endpoints for a
// new cluster after its CDS is installed and before its EDS is. The same update
// removes a cluster, so the endpoints still installed name one that the request
// no longer asks for, which an ADS cache would drop the request over. The EDS
// install that follows must answer it.
func TestADSCacheAnswersAnEndpointRequestBetweenInstalls(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(adsModeName(ordered), func(t *testing.T) {
			h := newADSHarness(t, ordered)
			c, ok := h.cache.(*perTypeCache)
			require.True(t, ok, "newSnapshotCache must return a perTypeCache")
			p := newADSProxy(h)
			h.setSnapshot(clusterEndpointSnapshot("1", "1", []string{"backend", "retired"}, map[string]string{"backend": "10.0.0.1", "retired": "10.0.0.9"}))
			p.subscribe(envoyresource.ClusterType)
			p.accept(p.next(envoyresource.ClusterType))
			p.subscribe(envoyresource.EndpointType, "backend", "retired")
			require.True(t, p.await(envoyresource.EndpointType, map[string]string{"backend": "10.0.0.1", "retired": "10.0.0.9"}), "initial endpoints")
			p.h.waitForOpenWatches(2)

			// Version 2 replaces retired with added. As soon as its CDS is installed,
			// Envoy applies it and requests endpoints for the clusters it now has.
			next := clusterEndpointSnapshot("2", "2", []string{"backend", "added"}, map[string]string{"backend": "10.0.0.11", "added": "10.0.0.2"})
			require.NoError(t, c.setSnapshot(h.ctx, adsTestNodeID, next, func(typeURL string) {
				if typeURL != envoyresource.ClusterType {
					return
				}
				p.accept(p.next(envoyresource.ClusterType))
				p.subscribeHandled(envoyresource.EndpointType, "added", "backend")
			}))
			want := map[string]string{"backend": "10.0.0.11", "added": "10.0.0.2"}
			require.Truef(t, p.await(envoyresource.EndpointType, want),
				"the proxy has endpoints %v, want %v: the endpoint request that arrived between the CDS and EDS installs was not answered",
				p.received[envoyresource.EndpointType], want)
		})
	}
}

// The listener counterpart: Envoy requests routes for a new listener after its
// LDS is installed and before its RDS is, while the routes still installed name
// a removed listener's RouteConfiguration. The RDS install must answer it.
func TestADSCacheAnswersARouteRequestBetweenInstalls(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(adsModeName(ordered), func(t *testing.T) {
			h := newADSHarness(t, ordered)
			c, ok := h.cache.(*perTypeCache)
			require.True(t, ok, "newSnapshotCache must return a perTypeCache")
			p := newADSProxy(h)
			h.setSnapshot(listenerRouteSnapshot("1", "1", []string{"front", "retired"}, map[string]string{"front-routes": "backend-v1", "retired-routes": "backend-v1"}))
			p.subscribe(envoyresource.ListenerType)
			p.accept(p.next(envoyresource.ListenerType))
			p.subscribe(envoyresource.RouteType, "front-routes", "retired-routes")
			require.True(t, p.await(envoyresource.RouteType, map[string]string{"front-routes": "backend-v1", "retired-routes": "backend-v1"}), "initial routes")
			p.h.waitForOpenWatches(2)

			next := listenerRouteSnapshot("2", "2", []string{"front", "added"}, map[string]string{"front-routes": "backend-v2", "added-routes": "backend-v1"})
			require.NoError(t, c.setSnapshot(h.ctx, adsTestNodeID, next, func(typeURL string) {
				if typeURL != envoyresource.ListenerType {
					return
				}
				p.accept(p.next(envoyresource.ListenerType))
				p.subscribeHandled(envoyresource.RouteType, "added-routes", "front-routes")
			}))
			want := map[string]string{"front-routes": "backend-v2", "added-routes": "backend-v1"}
			require.Truef(t, p.await(envoyresource.RouteType, want),
				"the proxy has routes %v, want %v: the route request that arrived between the LDS and RDS installs was not answered",
				p.received[envoyresource.RouteType], want)
		})
	}
}
