package proxy_syncer

import (
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

// The EDS churn benchmarks price an endpoint change for a gateway with many
// EDS backends. Each change replaces one backend's CLA row for every client,
// and every client's EDS set and snapshot are reassembled from all its rows,
// then published. The burst variant changes many backends back to back, as a
// rollout or the initial EndpointSlice sync does.
type edsChurnFixture struct {
	clients   []ir.UniquelyConnectedClient
	backends  []string
	endpoints krt.StaticCollection[UccWithEndpoints]
	snapshots krt.Collection[XdsSnapWrapper]
	events    atomic.Int64
	gen       uint64
}

func newEDSChurnFixture(b *testing.B, numClients, numBackends int) *edsChurnFixture {
	b.Helper()
	disarmTripwire(b)
	stop := make(chan struct{})
	b.Cleanup(func() { close(stop) })
	krtopts := krtutil.NewKrtOptions(stop, nil)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	f := &edsChurnFixture{}
	for i := range numClients {
		f.clients = append(f.clients, ir.NewUniquelyConnectedClient(role, "pod-ns", map[string]string{"client": strconv.Itoa(i)}, ir.PodLocality{}))
	}
	for i := range numBackends {
		f.backends = append(f.backends, fmt.Sprintf("kube_ns_svc-%d_8080", i))
	}

	var clusterRows []uccWithCluster
	var endpointRows []UccWithEndpoints
	for _, ucc := range f.clients {
		for _, backend := range f.backends {
			clusterRows = append(clusterRows, edsClusterForClient(ucc, backend, 1))
			endpointRows = append(endpointRows, endpointsForClient(ucc, backend, 1))
		}
	}
	uccs := krt.NewStaticCollection(nil, f.clients)
	routes := routeResourcesForClusters(f.backends...)
	listeners := sliceToResources([]*envoylistenerv3.Listener{{Name: "listener"}})
	mostXdsSnapshots := krt.NewStaticCollection(nil, []GatewayXdsResources{{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             routes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(routes, listeners),
	}})
	f.endpoints = krt.NewStaticCollection(nil, endpointRows)
	clusters, _ := newTestPerClientClusters(clusterRows)
	f.snapshots = snapshotPerClient(
		krtopts,
		uccs,
		mostXdsSnapshots,
		PerClientEnvoyEndpoints{
			endpoints: f.endpoints,
			index: krtpkg.UnnamedIndex(f.endpoints, func(ep UccWithEndpoints) []string {
				return []string{ep.Client.ResourceName()}
			}),
		},
		clusters,
		clusterScoping{},
	)
	translator := NewProxyTranslator(envoycache.NewSnapshotCache(true, envoycache.IDHash{}, nil), stubPriorXDS{has: true}, time.Hour, false, clusterScoping{})
	registerSyncXds(f.snapshots, translator)
	f.snapshots.RegisterBatch(func(events []krt.Event[XdsSnapWrapper]) {
		f.events.Add(int64(len(events)))
	}, false)

	f.waitFor(b, f.backends[len(f.backends)-1], 1)
	return f
}

// update replaces backend's CLA row for every client and returns the rows' new
// hash. Hashes are mixed so that the XOR set version of a burst cannot cancel
// out, as small sequential values do.
func (f *edsChurnFixture) update(backend string) uint64 {
	f.gen++
	hash := splitmix64(f.gen)
	for _, ucc := range f.clients {
		f.endpoints.UpdateObject(endpointsForClient(ucc, backend, hash))
	}
	return hash
}

func splitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

// waitFor polls until every client's snapshot carries hash for backend.
func (f *edsChurnFixture) waitFor(b *testing.B, backend string, hash uint64) {
	b.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for _, ucc := range f.clients {
		for {
			snap := f.snapshots.GetKey(ucc.ResourceName())
			if snap != nil && snap.endpointHashes[backend] == hash {
				break
			}
			if time.Now().After(deadline) {
				b.Fatalf("client %s never saw %s at %d", ucc.ResourceName(), backend, hash)
			}
			time.Sleep(50 * time.Microsecond)
		}
	}
}

func edsChurnShapes() []struct{ clients, backends int } {
	return []struct{ clients, backends int }{{1, 2000}, {8, 2000}}
}

// BenchmarkPerClientEDSChurnSingle changes one backend's endpoints and waits
// for every client's snapshot to carry the change.
func BenchmarkPerClientEDSChurnSingle(b *testing.B) {
	for _, shape := range edsChurnShapes() {
		b.Run(fmt.Sprintf("clients=%d/backends=%d", shape.clients, shape.backends), func(b *testing.B) {
			f := newEDSChurnFixture(b, shape.clients, shape.backends)
			f.events.Store(0)
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				backend := f.backends[i%len(f.backends)]
				f.waitFor(b, backend, f.update(backend))
			}
			b.ReportMetric(float64(f.events.Load())/float64(b.N), "snapshots/op")
		})
	}
}

// BenchmarkPerClientEDSChurnBurst changes burst backends back to back and waits
// for every client's snapshot to carry the last change.
func BenchmarkPerClientEDSChurnBurst(b *testing.B) {
	const burst = 100
	for _, shape := range edsChurnShapes() {
		b.Run(fmt.Sprintf("clients=%d/backends=%d", shape.clients, shape.backends), func(b *testing.B) {
			f := newEDSChurnFixture(b, shape.clients, shape.backends)
			f.events.Store(0)
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				var last string
				var hash uint64
				for j := range burst {
					last = f.backends[(i*burst+j)%len(f.backends)]
					hash = f.update(last)
				}
				f.waitFor(b, last, hash)
			}
			b.ReportMetric(float64(f.events.Load())/float64(b.N), "snapshots/op")
		})
	}
}
