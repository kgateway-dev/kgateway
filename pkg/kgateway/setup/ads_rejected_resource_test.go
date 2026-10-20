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
// response. Envoy applies the valid resources of that response anyway and never
// subscribes to the rejected resource's endpoints or routes. In ADS mode the
// cache answers a named EDS or RDS request only when the snapshot holds nothing
// outside the request, so:
//
//   - while the rejected resource is in the snapshot, its CLA or
//     RouteConfiguration holds back every endpoint or route update for the
//     proxy, not only its own;
//   - the request Envoy sends for a valid resource added in the same response
//     is declined on the request path, which registers no watch. Removing the
//     rejected resource then has nothing to answer, and the proxy stays on its
//     old endpoints or routes until its subscription changes again or it
//     reconnects.
//
// The request sequences are the ones Envoy v1.39.4 sent to this pinned
// go-control-plane with ordered ADS, with an EDS cluster rejected for
// CLUSTER_PROVIDED load balancing or an unrecognized TLS signature algorithm,
// and a listener rejected for an HTTP filter with no registered factory. In those runs the added cluster started empty at
// its 15s EDS fetch timeout, the added listener started with no routes at its
// RDS fetch timeout, and the existing cluster and route kept their first
// version through two later updates and the rejected resource's removal.
//
// Secrets are not affected: newSnapshotCache serves them per request
// (TestADSCacheServesRequestedSecrets).
//
// These tests pin dependency behavior. A go-control-plane that answers named
// ADS requests with only the subscribed resources fails all of them. One that
// keeps the watch of a request it declines fails only the "after it is
// removed" assertions. Either way, invert the failing assertions and keep the
// scenario.

const silenceWindow = 300 * time.Millisecond

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

func endpointAddresses(t *testing.T, resp *discoveryv3.DiscoveryResponse) map[string]string {
	t.Helper()

	out := map[string]string{}
	for _, r := range resp.GetResources() {
		cla := &envoyendpointv3.ClusterLoadAssignment{}
		require.NoError(t, r.UnmarshalTo(cla))
		out[cla.GetClusterName()] = cla.GetEndpoints()[0].GetLbEndpoints()[0].GetEndpoint().GetAddress().GetSocketAddress().GetAddress()
	}
	return out
}

func routeTargets(t *testing.T, resp *discoveryv3.DiscoveryResponse) map[string]string {
	t.Helper()

	out := map[string]string{}
	for _, r := range resp.GetResources() {
		rc := &envoyroutev3.RouteConfiguration{}
		require.NoError(t, r.UnmarshalTo(rc))
		out[rc.GetName()] = rc.GetVirtualHosts()[0].GetRoutes()[0].GetRoute().GetCluster()
	}
	return out
}

// request sends a request for names at the version Envoy holds, with the nonce
// of the last response it received for the type.
func (h *adsHarness) request(typeURL, version, nonce string, names ...string) {
	h.t.Helper()

	h.send(&discoveryv3.DiscoveryRequest{
		Node:          h.node,
		TypeUrl:       typeURL,
		ResourceNames: names,
		VersionInfo:   version,
		ResponseNonce: nonce,
	})
}

// rejectResponse NACKs resp, reporting accepted as the version Envoy holds.
func (h *adsHarness) rejectResponse(resp *discoveryv3.DiscoveryResponse, accepted string) {
	h.t.Helper()

	h.send(&discoveryv3.DiscoveryRequest{
		Node:          h.node,
		TypeUrl:       resp.GetTypeUrl(),
		VersionInfo:   accepted,
		ResponseNonce: resp.GetNonce(),
		ErrorDetail:   &status.Status{Message: "rejected by test"},
	})
}

// requireNoWatches requires that the cache holds no watch for the proxy, for
// example because the server cancelled the parked one to handle a request the
// cache then declined.
func (h *adsHarness) requireNoWatches(reason string) {
	h.t.Helper()

	require.Eventually(h.t, func() bool {
		info := h.cache.GetStatusInfo(adsTestNodeID)
		return info != nil && info.GetNumWatches() == 0
	}, 2*time.Second, 10*time.Millisecond, reason)
}

func (h *adsHarness) requireSilence(reason string) {
	h.t.Helper()

	select {
	case resp := <-h.stream.sent:
		h.t.Fatalf("%s: got a %s response at version %q", reason, resp.GetTypeUrl(), resp.GetVersionInfo())
	case err := <-h.done:
		require.NoError(h.t, err)
	case <-time.After(silenceWindow):
	}
}

// startWithBackend brings the proxy to one accepted EDS cluster, backend at
// 10.0.0.1, with its CDS and EDS watches parked. It returns the CDS and EDS
// responses the proxy accepted.
func startWithBackend(h *adsHarness) (*discoveryv3.DiscoveryResponse, *discoveryv3.DiscoveryResponse) {
	h.t.Helper()

	h.setSnapshot(clusterEndpointSnapshot("1", "1", []string{"backend"}, map[string]string{"backend": "10.0.0.1"}))
	h.subscribe(envoyresource.ClusterType)
	cds := h.receiveType(envoyresource.ClusterType, "1")
	h.subscribe(envoyresource.EndpointType, "backend")
	eds := h.receiveType(envoyresource.EndpointType, "1")
	require.Equal(h.t, map[string]string{"backend": "10.0.0.1"}, endpointAddresses(h.t, eds))
	h.ack(cds)
	h.ack(eds, "backend")
	h.waitForOpenWatches(2)
	return cds, eds
}

// addRejectedCluster publishes cluster "added" and cluster "rejected", which
// Envoy rejects, and sends what Envoy sends next: an EDS request for the
// clusters it applied, at its held version. Envoy acknowledges the CDS response
// only after "added" warms. It returns the CDS response Envoy will NACK.
func addRejectedCluster(h *adsHarness, eds *discoveryv3.DiscoveryResponse) *discoveryv3.DiscoveryResponse {
	h.t.Helper()

	h.setSnapshot(clusterEndpointSnapshot("2", "2",
		[]string{"backend", "added", "rejected"},
		map[string]string{"backend": "10.0.0.1", "added": "10.0.0.2", "rejected": "10.0.0.3"}))
	cds := h.receiveType(envoyresource.ClusterType, "2")
	h.requireSilence("the parked EDS watch must not be answered while the snapshot holds endpoints the proxy did not request")

	h.request(envoyresource.EndpointType, eds.GetVersionInfo(), eds.GetNonce(), "added", "backend")
	h.requireSilence("the EDS request for the applied clusters is declined because of the rejected cluster's endpoints")
	return cds
}

// While a cluster the proxy rejected is in the snapshot, the proxy gets no
// endpoint update for any of its clusters. A go-control-plane that answers
// named ADS requests with only the subscribed resources would deliver them.
func TestADSCacheRejectedClusterWithholdsEndpointsOfEveryCluster(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(adsModeName(ordered), func(t *testing.T) {
			h := newADSHarness(t, ordered)
			_, eds := startWithBackend(h)
			addRejectedCluster(h, eds)

			h.setSnapshot(clusterEndpointSnapshot("2", "3",
				[]string{"backend", "added", "rejected"},
				map[string]string{"backend": "10.0.0.11", "added": "10.0.0.2", "rejected": "10.0.0.3"}))
			h.requireSilence("backend's new endpoints are withheld because of a cluster the proxy rejected")
		})
	}
}

// Removing the rejected cluster does not release the proxy: the declined EDS
// request left no watch, and Envoy's EDS subscription does not change, so no
// endpoint update reaches it until a later CDS change adds or removes an EDS
// cluster. A go-control-plane that keeps the watch of a request it declines
// would answer as soon as the rejected cluster is gone.
func TestADSCacheRejectedClusterStrandsEndpointsAfterItIsRemoved(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(adsModeName(ordered), func(t *testing.T) {
			h := newADSHarness(t, ordered)
			_, eds := startWithBackend(h)
			rejected := addRejectedCluster(h, eds)
			h.requireNoWatches("the declined EDS request must leave no watch; a go-control-plane that keeps it fails here")

			// kgateway drops the rejected cluster and moves backend; Envoy then
			// NACKs the CDS response that carried it and accepts the next one.
			h.setSnapshot(clusterEndpointSnapshot("3", "4",
				[]string{"backend", "added"},
				map[string]string{"backend": "10.0.0.11", "added": "10.0.0.2"}))
			h.rejectResponse(rejected, "1")
			fixed := h.receiveType(envoyresource.ClusterType, "3")
			h.ack(fixed)
			h.waitForOpenWatches(1)
			h.requireSilence("the snapshot is answerable again, but the declined EDS request left no watch to answer")

			h.setSnapshot(clusterEndpointSnapshot("3", "5",
				[]string{"backend", "added"},
				map[string]string{"backend": "10.0.0.21", "added": "10.0.0.22"}))
			h.requireSilence("later endpoint changes are stranded too")

			// Only a change to the EDS subscription recovers the proxy.
			h.setSnapshot(clusterEndpointSnapshot("4", "6",
				[]string{"backend", "added", "later"},
				map[string]string{"backend": "10.0.0.21", "added": "10.0.0.22", "later": "10.0.0.23"}))
			h.ack(h.receiveType(envoyresource.ClusterType, "4"))
			h.request(envoyresource.EndpointType, eds.GetVersionInfo(), eds.GetNonce(), "added", "backend", "later")
			recovered := h.receiveType(envoyresource.EndpointType, "6")
			require.Equal(t, map[string]string{"backend": "10.0.0.21", "added": "10.0.0.22", "later": "10.0.0.23"}, endpointAddresses(t, recovered))
		})
	}
}

// The listener counterpart: a listener the proxy rejected holds back every
// route update while its RouteConfiguration is in the snapshot, and the RDS
// request for the routes the proxy did accept leaves no watch, so routes stay
// stranded after the listener is removed.
func TestADSCacheRejectedListenerStrandsRoutesAfterItIsRemoved(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(adsModeName(ordered), func(t *testing.T) {
			h := newADSHarness(t, ordered)

			h.setSnapshot(listenerRouteSnapshot("1", "1", []string{"front"}, map[string]string{"front-routes": "backend-v1"}))
			h.subscribe(envoyresource.ListenerType)
			lds := h.receiveType(envoyresource.ListenerType, "1")
			h.subscribe(envoyresource.RouteType, "front-routes")
			rds := h.receiveType(envoyresource.RouteType, "1")
			require.Equal(t, map[string]string{"front-routes": "backend-v1"}, routeTargets(t, rds))
			h.ack(lds)
			h.ack(rds, "front-routes")
			h.waitForOpenWatches(2)

			// Listener "added" and listener "rejected", which Envoy rejects.
			h.setSnapshot(listenerRouteSnapshot("2", "2",
				[]string{"front", "added", "rejected"},
				map[string]string{"front-routes": "backend-v1", "added-routes": "backend-v1", "rejected-routes": "backend-v1"}))
			rejected := h.receiveType(envoyresource.ListenerType, "2")
			h.requireSilence("the parked RDS watch must not be answered while the snapshot holds routes the proxy did not request")
			h.request(envoyresource.RouteType, rds.GetVersionInfo(), rds.GetNonce(), "added-routes", "front-routes")
			h.requireSilence("the RDS request for the applied listeners is declined because of the rejected listener's routes")
			h.requireNoWatches("the declined RDS request must leave no watch; a go-control-plane that keeps it fails here")

			h.setSnapshot(listenerRouteSnapshot("2", "3",
				[]string{"front", "added", "rejected"},
				map[string]string{"front-routes": "backend-v2", "added-routes": "backend-v1", "rejected-routes": "backend-v1"}))
			h.requireSilence("front's route change is withheld because of a listener the proxy rejected")

			// kgateway drops the rejected listener; Envoy NACKs the LDS response
			// that carried it and accepts the next one.
			h.setSnapshot(listenerRouteSnapshot("3", "4",
				[]string{"front", "added"},
				map[string]string{"front-routes": "backend-v2", "added-routes": "backend-v1"}))
			h.rejectResponse(rejected, "1")
			h.ack(h.receiveType(envoyresource.ListenerType, "3"))
			h.waitForOpenWatches(1)
			h.requireSilence("the snapshot is answerable again, but the declined RDS request left no watch to answer")

			// Only a change to the RDS subscription recovers the proxy.
			h.setSnapshot(listenerRouteSnapshot("4", "5",
				[]string{"front", "added", "later"},
				map[string]string{"front-routes": "backend-v2", "added-routes": "backend-v1", "later-routes": "backend-v1"}))
			h.ack(h.receiveType(envoyresource.ListenerType, "4"))
			h.request(envoyresource.RouteType, rds.GetVersionInfo(), rds.GetNonce(), "added-routes", "front-routes", "later-routes")
			recovered := h.receiveType(envoyresource.RouteType, "5")
			require.Equal(t, "backend-v2", routeTargets(t, recovered)["front-routes"])
		})
	}
}
