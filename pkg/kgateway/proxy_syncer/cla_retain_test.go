package proxy_syncer

import (
	"fmt"
	"testing"
	"time"

	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/endpoints"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// sequentialClaFixture builds the endpoints collection against a UCC collection
// that starts empty, so clients can be connected one at a time. That arrival
// order is the point: interning that only works when every client is present in
// a single recomputation does not work at all for a fleet, because proxies
// connect one by one.
type sequentialClaFixture struct {
	t          *testing.T
	uccs       krt.StaticCollection[ir.UniquelyConnectedClient]
	endpoints  krt.StaticCollection[ir.EndpointsForBackend]
	perClient  PerClientEnvoyEndpoints
	backendRef ir.BackendObjectIR
}

func newSequentialClaFixture(t *testing.T) *sequentialClaFixture {
	t.Helper()
	krtopts := krtutil.NewKrtOptions(t.Context().Done(), nil)

	backend := ir.NewBackendObjectIR(ir.ObjectSource{
		Kind: "Service", Namespace: "default", Name: "backend",
	}, 80, "", "")

	f := &sequentialClaFixture{t: t, backendRef: backend}
	f.uccs = krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{}, krtopts.ToOptions("UniqueClients")...)
	f.endpoints = krt.NewStaticCollection(nil, []ir.EndpointsForBackend{*f.endpointsWith("original")}, krtopts.ToOptions("Endpoints")...)

	f.perClient = NewPerClientEnvoyEndpoints(
		krtopts,
		f.uccs,
		f.endpoints,
		func(_ krt.HandlerContext, _ ir.UniquelyConnectedClient, ep ir.EndpointsForBackend) translator.ResolvedEndpoints {
			return translator.ResolvedEndpoints{Inputs: endpoints.EndpointsInputs{EndpointsForBackend: ep}}
		},
		func(ucc ir.UniquelyConnectedClient, resolved translator.ResolvedEndpoints) *envoyendpointv3.ClusterLoadAssignment {
			return endpoints.PrioritizeEndpoints(nil, ucc, resolved.Inputs)
		},
	)
	return f
}

// endpointsWith builds the backend's endpoints with one addressable endpoint, so
// changing the address changes the resolved content and therefore the hash.
func (f *sequentialClaFixture) endpointsWith(address string) *ir.EndpointsForBackend {
	eps := ir.NewEndpointsForBackend(f.backendRef)
	eps.Add(ir.PodLocality{}, ir.EndpointWithMd{LbEndpoint: lbEndpointPipe(address)})
	return eps
}

// connect adds one client and waits for its row to be stored, so the next
// connection is genuinely a separate recomputation rather than a batched one.
func (f *sequentialClaFixture) connect(name string) ir.UniquelyConnectedClient {
	f.t.Helper()
	ucc := ir.NewUniquelyConnectedClient(name, "ns", nil, ir.PodLocality{})
	f.uccs.UpdateObject(ucc)
	require.Eventually(f.t, func() bool {
		return len(f.perClient.FetchEndpointsForClient(krt.TestingDummyContext{}, ucc)) == 1
	}, time.Second, 20*time.Millisecond, "row for client %q should be stored before the next connects", name)
	return ucc
}

func (f *sequentialClaFixture) row(ucc ir.UniquelyConnectedClient) UccWithEndpoints {
	f.t.Helper()
	rows := f.perClient.FetchEndpointsForClient(krt.TestingDummyContext{}, ucc)
	require.Len(f.t, rows, 1, "expected exactly one row for %q", ucc.ResourceName())
	return rows[0]
}

// Clients that connect one at a time must converge on a single CLA proto. Before
// the interner was carried across recomputations this retained one proto per
// client: the newcomer got the freshly built proto, and every already-connected
// client kept the one from the pass it joined in, because KRT keeps the object
// it already stored when Equals reports no change.
func TestNewPerClientEnvoyEndpointsSharesClaAcrossSequentialConnections(t *testing.T) {
	f := newSequentialClaFixture(t)

	a := f.connect("a")
	b := f.connect("b")
	c := f.connect("c")

	rowA, rowB, rowC := f.row(a), f.row(b), f.row(c)
	require.Equal(t, rowA.EndpointsHash, rowB.EndpointsHash, "precondition: equivalent clients must resolve alike")
	require.Equal(t, rowA.EndpointsHash, rowC.EndpointsHash, "precondition: equivalent clients must resolve alike")

	require.True(t, sharedproto.Same(rowA.Endpoints, rowB.Endpoints),
		"a client that connects later must be given the proto the stored rows already reference")
	require.True(t, sharedproto.Same(rowB.Endpoints, rowC.Endpoints),
		"interning must keep converging as more clients connect, not just for the first pair")
}

// Sharing has to survive a longer arrival sequence too: one proto for the whole
// fleet, not one per arrival cohort.
func TestNewPerClientEnvoyEndpointsSharesOneClaAcrossManyConnections(t *testing.T) {
	f := newSequentialClaFixture(t)

	const clients = 12
	connected := make([]ir.UniquelyConnectedClient, 0, clients)
	for i := range clients {
		connected = append(connected, f.connect(fmt.Sprintf("client-%02d", i)))
	}

	first := f.row(connected[0])
	for _, ucc := range connected[1:] {
		require.True(t, sharedproto.Same(first.Endpoints, f.row(ucc).Endpoints),
			"every equivalent client must share one CLA proto, however many connected before it")
	}
}

// When the backend's endpoints change, every client must move to the newly built
// proto. This is the other half of the bounding argument: the superseded proto
// must stop being handed out, or the retained set would grow by one generation
// per endpoint change - which for a churning backend is worse than not interning.
func TestNewPerClientEnvoyEndpointsRetiresSupersededClaOnEndpointChange(t *testing.T) {
	f := newSequentialClaFixture(t)

	a := f.connect("a")
	b := f.connect("b")
	before := f.row(a)
	require.True(t, sharedproto.Same(before.Endpoints, f.row(b).Endpoints), "precondition: clients start out sharing")

	f.endpoints.UpdateObject(*f.endpointsWith("changed"))
	require.Eventually(t, func() bool {
		return f.row(a).EndpointsHash != before.EndpointsHash
	}, time.Second, 20*time.Millisecond, "the endpoint change should reach the rows")

	afterA, afterB := f.row(a), f.row(b)
	require.True(t, sharedproto.Same(afterA.Endpoints, afterB.Endpoints),
		"clients must still share one proto after the change")
	require.False(t, sharedproto.Same(before.Endpoints, afterA.Endpoints),
		"the superseded proto must not be handed out again once the content changed")

	// A client connecting after the change joins the current generation rather
	// than starting another one.
	c := f.connect("c")
	require.True(t, sharedproto.Same(afterA.Endpoints, f.row(c).Endpoints),
		"a client connecting after the change must join the current generation")
}

// A disconnect must not strand the remaining clients on separate protos.
func TestNewPerClientEnvoyEndpointsSharesClaAcrossDisconnect(t *testing.T) {
	f := newSequentialClaFixture(t)

	a := f.connect("a")
	b := f.connect("b")
	f.uccs.DeleteObject(a.ResourceName())
	require.Eventually(t, func() bool {
		return len(f.perClient.FetchEndpointsForClient(krt.TestingDummyContext{}, a)) == 0
	}, time.Second, 20*time.Millisecond, "the disconnected client's row should be dropped")

	c := f.connect("c")
	require.True(t, sharedproto.Same(f.row(b).Endpoints, f.row(c).Endpoints),
		"a client connecting after a disconnect must share with the clients still connected")
}

// The retainer holds exactly what the live rows reference. These assertions are
// on the retained set directly, because "one proto is shared" and "only one proto
// is kept alive" are different claims and only the second one bounds memory.
func TestCLARetainerKeepsOnlyWhatRowsReference(t *testing.T) {
	r := newCLARetainer()
	first := sharedproto.Wrap(&envoyendpointv3.ClusterLoadAssignment{ClusterName: "first"})
	second := sharedproto.Wrap(&envoyendpointv3.ClusterLoadAssignment{ClusterName: "second"})

	r.keep("backend", []UccWithEndpoints{
		{EndpointsHash: 1, Endpoints: first},
		{EndpointsHash: 1, Endpoints: first},
	})
	require.Len(t, r.byBackend["backend"], 1, "one shared proto across many rows must be retained once")

	// A new generation replaces the old one rather than accumulating beside it.
	r.keep("backend", []UccWithEndpoints{{EndpointsHash: 2, Endpoints: second}})
	require.Len(t, r.byBackend["backend"], 1, "a superseded generation must not be retained")
	require.True(t, sharedproto.Same(second, r.byBackend["backend"][0].cla))

	// Genuinely distinct results are both live, so both are retained.
	r.keep("backend", []UccWithEndpoints{
		{EndpointsHash: 1, Endpoints: first},
		{EndpointsHash: 2, Endpoints: second},
	})
	require.Len(t, r.byBackend["backend"], 2, "distinct live results must each be retained")

	// Rows without a proto contribute nothing to retention.
	r.keep("backend", []UccWithEndpoints{{EndpointsHash: 3}})
	require.Empty(t, r.byBackend["backend"], "a backend whose rows reference no proto must retain nothing")
}

// A deleted backend never runs the transform again, so its entry has to be
// dropped explicitly or it outlives every row that referenced it.
func TestCLARetainerForgetsDeletedBackend(t *testing.T) {
	r := newCLARetainer()
	cla := sharedproto.Wrap(&envoyendpointv3.ClusterLoadAssignment{ClusterName: "gone"})
	r.keep("backend", []UccWithEndpoints{{EndpointsHash: 1, Endpoints: cla}})
	require.Len(t, r.byBackend, 1)

	r.forget("backend")
	require.Empty(t, r.byBackend, "a deleted backend must not keep its CLAs alive")
}

// Seeding is what carries interning across recomputations: a candidate built in
// a later pass must be answered with the instance the earlier pass handed out.
func TestCLARetainerSeedsInternerWithLiveGeneration(t *testing.T) {
	r := newCLARetainer()
	original := sharedproto.Wrap(&envoyendpointv3.ClusterLoadAssignment{ClusterName: "shared"})
	r.keep("backend", []UccWithEndpoints{{EndpointsHash: 7, Endpoints: original}})

	var interner sharedproto.Interner[*envoyendpointv3.ClusterLoadAssignment]
	r.seed("backend", &interner)

	rebuilt := interner.Intern(&envoyendpointv3.ClusterLoadAssignment{ClusterName: "shared"}, 7)
	require.True(t, sharedproto.Same(original, rebuilt),
		"an equal candidate must be answered with the retained instance, not a new one")

	// Seeding narrows candidates by bucket but never decides equality on its own.
	different := interner.Intern(&envoyendpointv3.ClusterLoadAssignment{ClusterName: "other"}, 7)
	require.False(t, sharedproto.Same(original, different),
		"a colliding bucket must not alias a different CLA onto the retained one")

	// A backend with nothing retained seeds nothing.
	var empty sharedproto.Interner[*envoyendpointv3.ClusterLoadAssignment]
	r.seed("absent", &empty)
	fresh := empty.Intern(&envoyendpointv3.ClusterLoadAssignment{ClusterName: "shared"}, 7)
	require.False(t, sharedproto.Same(original, fresh))
}
