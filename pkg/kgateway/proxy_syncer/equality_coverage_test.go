package proxy_syncer

// equality_coverage_test.go covers the per-client and per-gateway xDS types whose
// Equals implementations summarize a payload with a hash instead of comparing the
// protos, and checks that representative payload changes move the hash that
// Equals compares.
//
// See test/testutils/equalstest for the harness API.

import (
	"errors"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/test/testutils/equalstest"
)

func testUcc() ir.UniquelyConnectedClient {
	return ir.NewUniquelyConnectedClient("gateway~default~my-gateway", "default",
		map[string]string{"app": "my-gateway"}, ir.PodLocality{Region: "us-east-1", Zone: "us-east-1a"})
}

// MARK: uccWithCluster

func baseUccWithCluster() uccWithCluster {
	cluster := &envoyclusterv3.Cluster{Name: "cluster-a"}
	return uccWithCluster{
		Client:            testUcc(),
		Cluster:           sharedproto.Wrap(cluster),
		ClusterVersion:    utils.HashProto(cluster),
		Name:              cluster.GetName(),
		Error:             nil,
		BackendSource:     ir.ObjectSource{Kind: "Service", Namespace: "default", Name: "svc-a"},
		BackendGeneration: 1,
	}
}

func TestHarnessUccWithClusterEquals(t *testing.T) {
	cases := []equalstest.Case[uccWithCluster]{
		{
			Field: "Client",
			Mutate: func(u *uccWithCluster) {
				u.Client = ir.NewUniquelyConnectedClient("gateway~default~other-gateway", "default",
					map[string]string{"app": "other-gateway"}, ir.PodLocality{})
			},
		},
		{
			Field:  "Name",
			Mutate: func(u *uccWithCluster) { u.Name = "cluster-b" },
		},
		{
			Field:  "ClusterVersion",
			Mutate: func(u *uccWithCluster) { u.ClusterVersion++ },
		},
		{
			Field:  "PerClientError",
			Mutate: func(u *uccWithCluster) { u.PerClientError = true },
		},
		{
			Field:  "Error",
			Mutate: func(u *uccWithCluster) { u.Error = errors.New("translation failed") },
		},
		{
			Field:  "BackendSource",
			Mutate: func(u *uccWithCluster) { u.BackendSource.Name = "svc-b" },
		},
		{
			Field:  "BackendGeneration",
			Mutate: func(u *uccWithCluster) { u.BackendGeneration = 2 },
		},
	}

	equalstest.Run(
		t,
		baseUccWithCluster,
		func(a, b uccWithCluster) bool { return a.Equals(b) },
		cases,
		// Cluster is +noKrtEquals: publishable content is covered by ClusterVersion; see
		// TestUccWithClusterEqualsObservesClusterChangeViaVersion.
		[]string{"Cluster"},
		equalstest.IncludeUnexported(),
	)
}

// TestUccWithClusterEqualsObservesClusterChangeViaVersion checks that Equals
// observes a changed proto through its version, using the successful overlay path's
// HashProto scheme. Shared bases use baseClusterVersion instead.
func TestUccWithClusterEqualsObservesClusterChangeViaVersion(t *testing.T) {
	orig := baseUccWithCluster()

	// Model a successful overlay returned by clustersForClient.
	changedCluster := &envoyclusterv3.Cluster{
		Name:           "cluster-a",
		ConnectTimeout: durationSeconds(5),
	}
	changed := baseUccWithCluster()
	changed.Cluster = sharedproto.Wrap(changedCluster)
	changed.ClusterVersion = utils.HashProto(changedCluster)

	if orig.Equals(changed) {
		t.Error("Equals returned true for a cluster whose proto (and so ClusterVersion) changed")
	}
}

// MARK: UccWithEndpoints

func baseUccWithEndpoints() UccWithEndpoints {
	cla := &envoyendpointv3.ClusterLoadAssignment{ClusterName: "cluster-a"}
	return UccWithEndpoints{
		Client:        testUcc(),
		Endpoints:     sharedproto.Wrap(cla),
		EndpointsHash: utils.HashProto(cla),
		endpointsName: "svc-a",
	}
}

func TestHarnessUccWithEndpointsEquals(t *testing.T) {
	cases := []equalstest.Case[UccWithEndpoints]{
		{
			Field: "Client",
			Mutate: func(u *UccWithEndpoints) {
				u.Client = ir.NewUniquelyConnectedClient("gateway~default~other-gateway", "default",
					map[string]string{"app": "other-gateway"}, ir.PodLocality{})
			},
		},
		{
			Field:  "EndpointsHash",
			Mutate: func(u *UccWithEndpoints) { u.EndpointsHash++ },
		},
		{
			Field:  "endpointsName",
			Mutate: func(u *UccWithEndpoints) { u.endpointsName = "svc-b" },
		},
	}

	equalstest.Run(
		t,
		baseUccWithEndpoints,
		func(a, b UccWithEndpoints) bool { return a.Equals(b) },
		cases,
		// Endpoints is +noKrtEquals: producers summarize CLA inputs or content in
		// EndpointsHash. resourceName caches the key derived from Client and endpointsName.
		[]string{"Endpoints", "resourceName"},
		equalstest.IncludeUnexported(),
	)
}

// TestUccWithEndpointsEqualsObservesEndpointChangeViaHash checks that Equals
// observes a changed EndpointsHash. This fixture uses HashProto; backend CLAs use
// combineEndpointHash, and local-cluster CLAs use hashLocalClusterLoadAssignment.
func TestUccWithEndpointsEqualsObservesEndpointChangeViaHash(t *testing.T) {
	orig := baseUccWithEndpoints()

	changedCla := &envoyendpointv3.ClusterLoadAssignment{
		ClusterName: "cluster-a",
		Endpoints: []*envoyendpointv3.LocalityLbEndpoints{{
			Priority: 1,
		}},
	}
	changed := baseUccWithEndpoints()
	changed.Endpoints = sharedproto.Wrap(changedCla)
	changed.EndpointsHash = utils.HashProto(changedCla)

	if orig.Equals(changed) {
		t.Error("Equals returned true for endpoints whose CLA (and so EndpointsHash) changed")
	}
}

// TestUccWithEndpointsEqualsCoversPluginContributions checks that changing only
// the plugin contribution passed to combineEndpointHash changes row equality.
func TestUccWithEndpointsEqualsCoversPluginContributions(t *testing.T) {
	ep := ir.NewEndpointsForBackend(ir.BackendObjectIR{})

	const pluginHashBefore, pluginHashAfter uint64 = 0, 0xabcdef

	before := UccWithEndpoints{
		Client:        testUcc(),
		EndpointsHash: combineEndpointHash(ep.LbEpsEqualityHash, pluginHashBefore, 0),
		endpointsName: ep.ResourceName(),
	}
	after := UccWithEndpoints{
		Client:        testUcc(),
		EndpointsHash: combineEndpointHash(ep.LbEpsEqualityHash, pluginHashAfter, 0),
		endpointsName: ep.ResourceName(),
	}

	if before.Equals(after) {
		t.Error("Equals returned true when only an endpoint plugin's contribution changed")
	}
}

// MARK: GatewayXdsResources

func baseGatewayXdsResources() GatewayXdsResources {
	clusters, clustersHash := sliceToResourcesHash([]*envoyclusterv3.Cluster{{Name: "extra-cluster"}})
	return GatewayXdsResources{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-gateway"},
		Clusters:       clusters,
		ClustersHash:   clustersHash,
		Routes:         sliceToResources([]*envoyroutev3.RouteConfiguration{{Name: "route-config"}}),
		Listeners:      sliceToResources([]*envoylistenerv3.Listener{{Name: "listener"}}),
		Secrets:        sliceToResources([]*envoytlsv3.Secret{{Name: "secret"}}),
	}
}

func TestHarnessGatewayXdsResourcesEquals(t *testing.T) {
	cases := []equalstest.Case[GatewayXdsResources]{
		{
			Field:  "NamespacedName",
			Mutate: func(r *GatewayXdsResources) { r.Name = "other-gateway" },
		},
		{
			Field:  "ClustersHash",
			Mutate: func(r *GatewayXdsResources) { r.ClustersHash++ },
		},
		{
			Field: "Routes",
			Mutate: func(r *GatewayXdsResources) {
				r.Routes = sliceToResources([]*envoyroutev3.RouteConfiguration{{Name: "other-route-config"}})
			},
		},
		{
			Field: "Listeners",
			Mutate: func(r *GatewayXdsResources) {
				r.Listeners = sliceToResources([]*envoylistenerv3.Listener{{Name: "other-listener"}})
			},
		},
		{
			Field: "Secrets",
			Mutate: func(r *GatewayXdsResources) {
				r.Secrets = sliceToResources([]*envoytlsv3.Secret{{Name: "other-secret"}})
			},
		},
	}

	equalstest.Run(
		t,
		baseGatewayXdsResources,
		func(a, b GatewayXdsResources) bool { return a.Equals(b) },
		cases,
		// Clusters is +noKrtEquals (covered by ClustersHash); Namespace/Name are the
		// flattened fields of the embedded NamespacedName, covered by that case.
		[]string{"Clusters", "Namespace", "Name"},
		equalstest.IncludeUnexported(),
	)
}

// TestGatewayXdsResourcesEqualsObservesClusterChangeViaHash is the evidence for the
// +noKrtEquals marker on Clusters: sliceToResourcesHash derives ClustersHash
// from the resource slice, and the changes exercised here move that hash.
func TestGatewayXdsResourcesEqualsObservesClusterChangeViaHash(t *testing.T) {
	orig := baseGatewayXdsResources()

	changed := baseGatewayXdsResources()
	changed.Clusters, changed.ClustersHash = sliceToResourcesHash([]*envoyclusterv3.Cluster{
		{Name: "extra-cluster", ConnectTimeout: durationSeconds(5)},
	})

	if orig.Equals(changed) {
		t.Error("Equals returned true for a CDS payload whose cluster (and so ClustersHash) changed")
	}

	// An added cluster must be observed too.
	added := baseGatewayXdsResources()
	added.Clusters, added.ClustersHash = sliceToResourcesHash([]*envoyclusterv3.Cluster{
		{Name: "extra-cluster"},
		{Name: "second-cluster"},
	})
	if orig.Equals(added) {
		t.Error("Equals returned true after a cluster was added to the CDS payload")
	}
}

func durationSeconds(s int64) *durationpb.Duration {
	return durationpb.New(time.Duration(s) * time.Second)
}
