package setup

import (
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	cachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
)

// rewarmCluster is an EDS cluster; its connect timeout stands in for any
// setting a policy changes without renaming the cluster.
func rewarmCluster(name string, connectTimeout time.Duration) *envoyclusterv3.Cluster {
	return &envoyclusterv3.Cluster{
		Name:                 name,
		ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS},
		ConnectTimeout:       durationpb.New(connectTimeout),
		EdsClusterConfig:     &envoyclusterv3.Cluster_EdsClusterConfig{ServiceName: name},
	}
}

// rewarmSnapshot serves one EDS cluster and its assignment, versioned
// separately so a test can change either without the other.
func rewarmSnapshot(clusterVersion, endpointVersion string, connectTimeout time.Duration) *envoycache.Snapshot {
	snap := snapshotFor(adsVersions{cluster: clusterVersion, endpoint: endpointVersion, listener: "1", route: "1"},
		[]*envoyclusterv3.Cluster{rewarmCluster(adsTestCluster, connectTimeout)}, nil)
	snap.Resources[cachetypes.Endpoint] = envoycache.NewResources(endpointVersion, []cachetypes.Resource{
		&envoyendpointv3.ClusterLoadAssignment{ClusterName: adsTestCluster},
	})
	return snap
}

// warmRewarmStream subscribes to clusters and to the cluster's endpoints and
// ACKs both, like a proxy whose cluster is warm. It returns the endpoint
// response: the proxy's next endpoint request repeats its version and nonce.
func warmRewarmStream(h *adsHarness, clusterVersion, endpointVersion string) *discoveryv3.DiscoveryResponse {
	h.t.Helper()

	h.setSnapshot(rewarmSnapshot(clusterVersion, endpointVersion, time.Second))
	h.subscribeAndAck(envoyresource.ClusterType, clusterVersion, 1)
	h.send(&discoveryv3.DiscoveryRequest{Node: h.node, TypeUrl: envoyresource.EndpointType, ResourceNames: []string{adsTestCluster}})
	eds := h.receiveType(envoyresource.EndpointType, endpointVersion)
	h.ackNamed(eds, adsTestCluster)
	h.waitForOpenWatches(2)
	return eds
}

// On the plain cache, a cluster changed while its endpoints keep their version
// never gets them. Envoy re-requests at the version it holds when the new
// cluster subscribes, so the cluster warms only at its fetch timeout, with CDS
// paused. Pinned so a go-control-plane change is noticed.
func TestPlainCacheLeavesRewarmedClusterEndpointRequestUnanswered(t *testing.T) {
	h := newADSHarness(t, true)
	eds := warmRewarmStream(h, "c1", "e1")

	h.setSnapshot(rewarmSnapshot("c2", "e1", 2*time.Second))
	h.ack(h.receiveType(envoyresource.ClusterType, "c2"))
	h.ackNamed(eds, adsTestCluster)
	h.expectSilence("endpoint request of a re-defined cluster on the plain cache")
}

// The policy answers that request with the cluster's assignment, once.
func TestRewarmedClusterEndpointRequestIsAnswered(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(adsModeName(ordered), func(t *testing.T) {
			h := newPolicyHarness(t, ordered, true)
			eds := warmRewarmStream(h, "c1", "e1")

			h.setSnapshot(rewarmSnapshot("c2", "e1", 2*time.Second))
			h.ack(h.receiveType(envoyresource.ClusterType, "c2"))
			h.ackNamed(eds, adsTestCluster)
			resp := h.receiveType(envoyresource.EndpointType, "e1")
			require.Len(t, resp.GetResources(), 1)

			h.ackNamed(resp, adsTestCluster)
			h.expectSilence("ACK of the re-warm response")
		})
	}
}

// The staging that left clusters warming in CI: the endpoint change reaches the
// proxy first and updates only the old cluster, then the cluster change
// re-defines it, and its request carries the version just received.
func TestRewarmAfterEndpointsMovedFirst(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(adsModeName(ordered), func(t *testing.T) {
			h := newPolicyHarness(t, ordered, true)
			warmRewarmStream(h, "c1", "e1")

			h.setSnapshot(rewarmSnapshot("c1", "e2", time.Second))
			eds := h.receiveType(envoyresource.EndpointType, "e2")
			h.ackNamed(eds, adsTestCluster)
			h.waitForOpenWatches(2)

			h.setSnapshot(rewarmSnapshot("c2", "e2", 2*time.Second))
			h.ack(h.receiveType(envoyresource.ClusterType, "c2"))
			h.ackNamed(eds, adsTestCluster)
			resp := h.receiveType(envoyresource.EndpointType, "e2")
			require.Len(t, resp.GetResources(), 1)
		})
	}
}

// A cluster response that only re-sends clusters unchanged re-defines nothing.
func TestRewarmIgnoresUnchangedClusters(t *testing.T) {
	h := newPolicyHarness(t, true, true)
	eds := warmRewarmStream(h, "c1", "e1")

	h.setSnapshot(rewarmSnapshot("c2", "e1", time.Second))
	h.ack(h.receiveType(envoyresource.ClusterType, "c2"))
	h.ackNamed(eds, adsTestCluster)
	h.expectSilence("endpoint request after an unchanged cluster was re-sent")
}

// When the endpoints move with the cluster, the ordered server sends the
// cluster first, and that endpoint response already re-warms it; the rule adds
// nothing.
func TestRewarmEndsWithAnEndpointResponseThatCarriesTheCluster(t *testing.T) {
	h := newPolicyHarness(t, true, true)
	warmRewarmStream(h, "c1", "e1")

	h.setSnapshot(rewarmSnapshot("c2", "e2", 2*time.Second))
	h.ack(h.receiveType(envoyresource.ClusterType, "c2"))
	eds := h.receiveType(envoyresource.EndpointType, "e2")
	h.ackNamed(eds, adsTestCluster)
	h.expectSilence("endpoint ACK after the endpoint response carried the re-defined cluster")
}

// Only the assignments an endpoint response carries are done: one built before
// the cluster response went out can carry just a newly subscribed name.
func TestRewarmEndsPerAssignmentSent(t *testing.T) {
	policy := newWatchPolicy(envoycache.NewSnapshotCache(true, envoycache.IDHash{}, nil), envoycache.IDHash{}, true)
	node := &envoycorev3.Node{Id: "node"}
	cdsRequest := &discoveryv3.DiscoveryRequest{Node: node, TypeUrl: envoyresource.ClusterType}
	edsRequest := &discoveryv3.DiscoveryRequest{Node: node, TypeUrl: envoyresource.EndpointType, ResourceNames: []string{"a", "b", "new"}}
	response := func(typeURL string, resources ...proto.Message) *discoveryv3.DiscoveryResponse {
		resp := &discoveryv3.DiscoveryResponse{TypeUrl: typeURL, VersionInfo: "1", Nonce: "1"}
		for _, resource := range resources {
			resp.Resources = append(resp.Resources, utils.MustMessageToAny(resource))
		}
		return resp
	}
	require.NoError(t, policy.onStreamRequest(1, cdsRequest))
	policy.onStreamResponse(t.Context(), 1, cdsRequest, response(envoyresource.ClusterType, rewarmCluster("a", time.Second), rewarmCluster("b", time.Second)))
	policy.onStreamResponse(t.Context(), 1, cdsRequest, response(envoyresource.ClusterType,
		rewarmCluster("a", 2*time.Second), rewarmCluster("b", 2*time.Second), rewarmCluster("new", time.Second)))
	require.Equal(t, map[string]struct{}{"a": {}, "b": {}}, policy.streams[1].rewarm, "a new cluster is not a re-definition")

	policy.onStreamResponse(t.Context(), 1, edsRequest, response(envoyresource.EndpointType,
		&envoyendpointv3.ClusterLoadAssignment{ClusterName: "new"}, &envoyendpointv3.ClusterLoadAssignment{ClusterName: "b"}))
	require.Equal(t, map[string]struct{}{"a": {}}, policy.streams[1].rewarm)

	policy.onStreamResponse(t.Context(), 1, cdsRequest, response(envoyresource.ClusterType, rewarmCluster("new", time.Second)))
	require.Empty(t, policy.streams[1].rewarm, "a removed cluster no longer warms")
}

// serviceCluster is an EDS cluster whose endpoints Envoy requests under
// service rather than its own name.
func serviceCluster(name, service string, connectTimeout time.Duration) *envoyclusterv3.Cluster {
	cluster := rewarmCluster(name, connectTimeout)
	cluster.EdsClusterConfig.ServiceName = service
	return cluster
}

// Marks are kept under the name Envoy requests a cluster's endpoints by, the
// EDS service_name when set, since that is what an endpoint response carries and
// the subscription records. A cluster that takes no endpoints is not marked, and
// a service shared by two re-defined clusters stays marked until both leave or an
// endpoint response carries it.
func TestRewarmTracksTheEndpointResourceName(t *testing.T) {
	policy := newWatchPolicy(envoycache.NewSnapshotCache(true, envoycache.IDHash{}, nil), envoycache.IDHash{}, true)
	node := &envoycorev3.Node{Id: "node"}
	cdsRequest := &discoveryv3.DiscoveryRequest{Node: node, TypeUrl: envoyresource.ClusterType}
	edsRequest := &discoveryv3.DiscoveryRequest{Node: node, TypeUrl: envoyresource.EndpointType}
	response := func(typeURL string, resources ...proto.Message) *discoveryv3.DiscoveryResponse {
		resp := &discoveryv3.DiscoveryResponse{TypeUrl: typeURL, VersionInfo: "1", Nonce: "1"}
		for _, resource := range resources {
			resp.Resources = append(resp.Resources, utils.MustMessageToAny(resource))
		}
		return resp
	}
	static := func(timeout time.Duration) *envoyclusterv3.Cluster {
		return &envoyclusterv3.Cluster{Name: "static", ConnectTimeout: durationpb.New(timeout)}
	}
	cds := func(timeout time.Duration, clusters ...string) *discoveryv3.DiscoveryResponse {
		var resources []proto.Message
		for _, name := range clusters {
			switch name {
			case "static":
				resources = append(resources, static(timeout))
			case "backend":
				resources = append(resources, serviceCluster("backend", "shared-service", timeout))
			default:
				resources = append(resources, serviceCluster(name, "shared", timeout))
			}
		}
		return response(envoyresource.ClusterType, resources...)
	}
	stream := func() *streamState { return policy.streams[1] }
	require.NoError(t, policy.onStreamRequest(1, cdsRequest))

	policy.onStreamResponse(t.Context(), 1, cdsRequest, cds(time.Second, "backend", "static"))
	policy.onStreamResponse(t.Context(), 1, cdsRequest, cds(2*time.Second, "backend", "static"))
	require.Equal(t, map[string]struct{}{"shared-service": {}}, stream().rewarm,
		"marked under its service name; a cluster without endpoint resources is not marked")

	policy.onStreamResponse(t.Context(), 1, edsRequest, response(envoyresource.EndpointType,
		&envoyendpointv3.ClusterLoadAssignment{ClusterName: "shared-service"}))
	require.Empty(t, stream().rewarm, "an endpoint response for the service ends the mark")
	require.Empty(t, stream().rewarmBy)

	policy.onStreamResponse(t.Context(), 1, cdsRequest, cds(3*time.Second, "backend"))
	require.Equal(t, map[string]struct{}{"shared-service": {}}, stream().rewarm)
	policy.onStreamResponse(t.Context(), 1, cdsRequest, cds(3*time.Second))
	require.Empty(t, stream().rewarm, "a removed cluster takes its mark with it")

	policy.onStreamResponse(t.Context(), 1, cdsRequest, cds(time.Second, "b1", "b2"))
	policy.onStreamResponse(t.Context(), 1, cdsRequest, cds(2*time.Second, "b1", "b2"))
	require.Equal(t, map[string]struct{}{"shared": {}}, stream().rewarm)
	policy.onStreamResponse(t.Context(), 1, cdsRequest, cds(2*time.Second, "b2"))
	require.Equal(t, map[string]struct{}{"shared": {}}, stream().rewarm, "the other cluster on the service still warms")
	policy.onStreamResponse(t.Context(), 1, cdsRequest, cds(2*time.Second))
	require.Empty(t, stream().rewarm)
}

// End to end in both ADS modes: a re-defined cluster whose endpoints are
// requested under a service name gets them at the held version.
func TestRewarmedClusterWithAServiceNameIsAnswered(t *testing.T) {
	const service = "shared-service"
	snapshot := func(clusterVersion string, connectTimeout time.Duration) *envoycache.Snapshot {
		snap := snapshotFor(adsVersions{cluster: clusterVersion, endpoint: "e1", listener: "1", route: "1"},
			[]*envoyclusterv3.Cluster{serviceCluster(adsTestCluster, service, connectTimeout)}, nil)
		snap.Resources[cachetypes.Endpoint] = envoycache.NewResources("e1", []cachetypes.Resource{
			&envoyendpointv3.ClusterLoadAssignment{ClusterName: service},
		})
		return snap
	}
	for _, ordered := range []bool{false, true} {
		t.Run(adsModeName(ordered), func(t *testing.T) {
			h := newPolicyHarness(t, ordered, true)
			h.setSnapshot(snapshot("c1", time.Second))
			h.subscribeAndAck(envoyresource.ClusterType, "c1", 1)
			h.send(&discoveryv3.DiscoveryRequest{Node: h.node, TypeUrl: envoyresource.EndpointType, ResourceNames: []string{service}})
			eds := h.receiveType(envoyresource.EndpointType, "e1")
			h.ackNamed(eds, service)
			h.waitForOpenWatches(2)

			h.setSnapshot(snapshot("c2", 2*time.Second))
			h.ack(h.receiveType(envoyresource.ClusterType, "c2"))
			h.ackNamed(eds, service)
			resp := h.receiveType(envoyresource.EndpointType, "e1")
			require.Len(t, resp.GetResources(), 1)

			h.ackNamed(resp, service)
			h.expectSilence("ACK of the re-warm response")
		})
	}
}

// The rule is per stream: a re-definition sent on one stream does not answer
// another stream's endpoint request.
func TestRewarmIsPerStream(t *testing.T) {
	policy := newWatchPolicy(envoycache.NewSnapshotCache(true, envoycache.IDHash{}, nil), envoycache.IDHash{}, true)
	node := &envoycorev3.Node{Id: "node"}
	cdsRequest := &discoveryv3.DiscoveryRequest{Node: node, TypeUrl: envoyresource.ClusterType}
	cds := func(timeout time.Duration) *discoveryv3.DiscoveryResponse {
		return &discoveryv3.DiscoveryResponse{TypeUrl: envoyresource.ClusterType, VersionInfo: "1", Nonce: "1", Resources: []*anypb.Any{utils.MustMessageToAny(rewarmCluster("a", timeout))}}
	}
	require.NoError(t, policy.onStreamRequest(1, cdsRequest))
	require.NoError(t, policy.onStreamRequest(2, cdsRequest))
	policy.onStreamResponse(t.Context(), 1, cdsRequest, cds(time.Second))
	policy.onStreamResponse(t.Context(), 2, cdsRequest, cds(time.Second))
	policy.onStreamResponse(t.Context(), 1, cdsRequest, cds(2*time.Second))

	other := &discoveryv3.DiscoveryRequest{Node: node, TypeUrl: envoyresource.EndpointType, ResourceNames: []string{"a"}, VersionInfo: "1", ResponseNonce: "1"}
	require.NoError(t, policy.onStreamRequest(2, other))
	require.NotContains(t, policy.rewarming, other)
	same := proto.Clone(other).(*discoveryv3.DiscoveryRequest)
	require.NoError(t, policy.onStreamRequest(1, same))
	require.Contains(t, policy.rewarming, same)

	policy.onStreamClosed(1, node)
	require.Empty(t, policy.rewarming, "closing the stream drops its association")
}

// resourceName reads field 1 of both resource types it is used on, however the
// rest is encoded.
func TestResourceNameReadsTheEncodedName(t *testing.T) {
	for _, resource := range []proto.Message{
		rewarmCluster("kube_default_svc_80", time.Second),
		&envoyendpointv3.ClusterLoadAssignment{ClusterName: "kube_default_svc_80", Endpoints: []*envoyendpointv3.LocalityLbEndpoints{{}}},
	} {
		encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(resource)
		require.NoError(t, err)
		name, ok := resourceName(encoded)
		require.True(t, ok)
		require.Equal(t, "kube_default_svc_80", name)
	}

	_, ok := resourceName([]byte{0xff})
	require.False(t, ok, "a malformed encoding has no name")
	encoded, err := proto.Marshal(&envoyclusterv3.Cluster{ConnectTimeout: durationpb.New(time.Second)})
	require.NoError(t, err)
	_, ok = resourceName(encoded)
	require.False(t, ok, "a cluster without a name has none")
}
