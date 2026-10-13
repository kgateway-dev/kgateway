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
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
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
		// Cluster is +noKrtEquals: publishable content is covered by ClusterVersion,
		// which the TestBaseClusterVersion_* tests derive from real cluster changes.
		[]string{"Cluster"},
	)
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

// TestUccWithEndpointsEqualsObservesEndpointChange builds EndpointsHash the way
// the backend CLA collection does, from an EndpointsForBackend through
// combineEndpointHash, and checks that an endpoint change alone is observed.
func TestUccWithEndpointsEqualsObservesEndpointChange(t *testing.T) {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "default", Name: "svc-a"}, 80, "", "")
	rowFor := func(address string) UccWithEndpoints {
		eps := ir.NewEndpointsForBackend(backend)
		eps.Add(ir.PodLocality{Region: "us-east-1"}, ir.EndpointWithMd{
			LbEndpoint: &envoyendpointv3.LbEndpoint{
				HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
					Endpoint: &envoyendpointv3.Endpoint{Hostname: address},
				},
			},
		})
		return UccWithEndpoints{
			Client:        testUcc(),
			EndpointsHash: combineEndpointHash(eps.LbEpsEqualityHash, 0, 0),
			endpointsName: eps.ResourceName(),
		}
	}

	if rowFor("10.0.0.1").Equals(rowFor("10.0.0.2")) {
		t.Error("Equals returned true for endpoints whose address changed")
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

	// A pair of duplicates must not cancel out of the hash.
	duplicated := baseGatewayXdsResources()
	duplicated.Clusters, duplicated.ClustersHash = sliceToResourcesHash([]*envoyclusterv3.Cluster{
		{Name: "extra-cluster"},
		{Name: "second-cluster"},
		{Name: "second-cluster"},
	})
	if orig.Equals(duplicated) {
		t.Error("Equals returned true after a duplicated pair of clusters was added to the CDS payload")
	}
}

func durationSeconds(s int64) *durationpb.Duration {
	return durationpb.New(time.Duration(s) * time.Second)
}

// MARK: EndpointsForBackend

// TestFinalBackendEndpointsEqualsObservesPolicyChange is the evidence for the
// +noKrtEquals marker on EndpointsForBackend.AttachedPolicies: it runs
// newFinalBackendEndpoints over backends that differ only in an attached
// policy's generation and checks that the resulting rows compare unequal.
func TestFinalBackendEndpointsEqualsObservesPolicyChange(t *testing.T) {
	krtopts := krtutil.NewKrtOptions(t.Context().Done(), nil)

	backendWithPolicyGen := func(gen int64) *ir.BackendObjectIR {
		backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "default", Name: "svc-a"}, 80, "", "")
		backend.AttachedPolicies = ir.AttachedPolicies{
			Policies: map[schema.GroupKind][]ir.PolicyAtt{
				{Group: "gateway.kgateway.dev", Kind: "BackendConfigPolicy"}: {{
					PolicyRef:  &ir.AttachedPolicyRef{Group: "gateway.kgateway.dev", Kind: "BackendConfigPolicy", Name: "pol", Namespace: "default"},
					Generation: gen,
				}},
			},
		}
		return &backend
	}

	finalRow := func(name string, backend *ir.BackendObjectIR) ir.EndpointsForBackend {
		raw := ir.NewEndpointsForBackend(*backend)
		raw.Add(ir.PodLocality{Region: "us-east-1"}, ir.EndpointWithMd{
			LbEndpoint: &envoyendpointv3.LbEndpoint{
				HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
					Endpoint: &envoyendpointv3.Endpoint{Hostname: "10.0.0.1"},
				},
			},
		})
		backends := krt.NewStaticCollection(nil, []*ir.BackendObjectIR{backend}, krtopts.ToOptions(name+"Backends")...)
		rawEndpoints := krt.NewStaticCollection(nil, []ir.EndpointsForBackend{*raw}, krtopts.ToOptions(name+"RawEndpoints")...)
		final := newFinalBackendEndpoints(krtopts, backends, rawEndpoints)

		var row *ir.EndpointsForBackend
		require.Eventually(t, func() bool {
			row = final.GetKey(backend.ResourceName())
			return row != nil
		}, 2*time.Second, 10*time.Millisecond)
		return *row
	}

	gen1 := finalRow("Gen1", backendWithPolicyGen(1))
	gen2 := finalRow("Gen2", backendWithPolicyGen(2))

	require.False(t, gen1.Equals(gen2),
		"a policy generation bump must change the final endpoints row even when the endpoints are unchanged")
}
