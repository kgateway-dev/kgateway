package proxy_syncer

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoydiscoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	envoystreamv3 "github.com/envoyproxy/go-control-plane/pkg/server/stream/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// connectionStartFixture drives the real client state's stream callbacks
// against a translator whose connection-start hooks are registered, over a
// real ADS snapshot cache.
type connectionStartFixture struct {
	closeStream func(sid int64)
	request     func(sid int64) error
	cache       envoycache.SnapshotCache
	translator  ProxyTranslator
	role        string
	current     atomic.Pointer[XdsSnapWrapper]
}

func newConnectionStartFixture(t *testing.T) *connectionStartFixture {
	t.Helper()
	// Read once per process; at worst each new stream below waits the 1s default.
	t.Setenv("KGW_XDS_FIRST_CONNECT_DELAY", "0s")
	ctx := t.Context()
	cb, uccBuilder, state := krtcollections.NewUniquelyConnectedClients(nil, false)
	uccs := uccBuilder(ctx, krtutil.KrtOptions{}, nil)
	uccs.WaitUntilSynced(ctx.Done())

	f := &connectionStartFixture{cache: newTestSnapshotCache(t), role: wellknown.GatewayApiProxyValue + "~ns~gw"}
	f.closeStream = func(sid int64) { cb.OnStreamClosed(sid, nil) }
	f.request = func(sid int64) error {
		return cb.OnStreamRequest(sid, &envoydiscoveryv3.DiscoveryRequest{
			Node: &envoycorev3.Node{Id: "podname.ns", Metadata: &structpb.Struct{Fields: map[string]*structpb.Value{
				xds.RoleKey: structpb.NewStringValue(f.role),
			}}},
			TypeUrl: resourcev3.ListenerType,
		})
	}
	f.translator = NewProxyTranslator(f.cache, state, time.Hour, true)
	require.True(t, f.translator.registerConnectionStartHooks(ctx, state, func(string) *XdsSnapWrapper { return f.current.Load() }))
	t.Cleanup(func() { f.translator.gate.clientDeparted(f.role) })
	return f
}

func (f *connectionStartFixture) build(version string) XdsSnapWrapper {
	snap := &envoycache.Snapshot{}
	snap.Resources[envoycachetypes.Listener] = envoycache.NewResources(version, nil)
	return XdsSnapWrapper{snap: snap, proxyKey: f.role}
}

func (f *connectionStartFixture) connect(t *testing.T, sid int64) {
	t.Helper()
	require.NoError(t, f.request(sid))
}

func (f *connectionStartFixture) served() (string, bool) {
	snap, err := f.cache.GetSnapshot(f.role)
	if err != nil {
		return "", false
	}
	return snap.GetVersion(resourcev3.ListenerType), true
}

// openListenerWatch opens the listener watch of a stream that holds version
// (empty for a newly connected stream's first request).
func (f *connectionStartFixture) openListenerWatch(t *testing.T, version string) chan envoycache.Response {
	t.Helper()
	responses := make(chan envoycache.Response, 1)
	_, err := f.cache.CreateWatch(&envoydiscoveryv3.DiscoveryRequest{
		Node:        &envoycorev3.Node{Id: f.role},
		TypeUrl:     resourcev3.ListenerType,
		VersionInfo: version,
	}, envoystreamv3.NewSotwSubscription(nil, true), responses)
	require.NoError(t, err)
	return responses
}

// A proxy that reconnects is never answered from the entry published to its
// previous connection: that entry can predate config it received meanwhile from
// another controller replica. The entry is cleared before the new stream is
// answered, and the client's current build is published to the new connection;
// with no current build the stream waits for the gate's first publish.
func TestReconnectIsNotServedThePreviousConnectionsEntry(t *testing.T) {
	f := newConnectionStartFixture(t)
	ctx := t.Context()

	f.connect(t, 1)
	f.translator.syncXds(ctx, f.build("v1"))
	version, ok := f.served()
	require.True(t, ok)
	require.Equal(t, "v1", version)
	f.closeStream(1)

	// The proxy returns; this replica's current build for it is v2.
	v2 := f.build("v2")
	f.current.Store(&v2)
	f.connect(t, 2)
	version, ok = f.served()
	require.True(t, ok)
	assert.Equal(t, "v2", version, "the new connection is answered from the current build, never the previous connection's entry")

	// Returning when there is no current build: the old entry is gone, so the
	// stream waits for the first publish instead of being answered from it.
	f.closeStream(2)
	f.current.Store(nil)
	f.connect(t, 3)
	_, ok = f.served()
	assert.False(t, ok, "the previous connection's entry must not be served")
}

// A publish computed for the previous connection must not reach the one that
// replaces it, even when it is already under way as the stream arrives: the
// publisher holds the gate lock, the new stream's admission clears the entry
// without it, and the publisher would otherwise install its build after the
// clear, where the new stream's first watch would find it.
func TestPublishRacingAReconnectIsNotInstalled(t *testing.T) {
	f := newConnectionStartFixture(t)
	ctx := t.Context()

	f.connect(t, 1)
	f.translator.syncXds(ctx, f.build("v1"))
	version, ok := f.served()
	require.True(t, ok)
	require.Equal(t, "v1", version)

	// A publish for the first connection takes the gate lock and is held just
	// before it installs.
	paused, resume := make(chan struct{}), make(chan struct{})
	var hold sync.Once
	f.translator.gate.beforeInstall = func(string) {
		hold.Do(func() {
			close(paused)
			<-resume
		})
	}
	published := make(chan struct{})
	go func() {
		defer close(published)
		f.translator.syncXds(ctx, f.build("old-on-first-connection"))
	}()
	<-paused

	// Meanwhile the stream closes and the proxy reconnects. Its current build
	// is deferred, so the new connection's first publish is withheld.
	f.closeStream(1)
	deferred := f.build("v2")
	deferred.missingReferenced = []string{"cluster-not-ready"}
	f.current.Store(&deferred)
	admitted := make(chan error, 1)
	go func() { admitted <- f.request(2) }()
	require.Eventually(t, func() bool {
		_, ok := f.served()
		return !ok
	}, 2*time.Second, time.Millisecond, "admitting the new stream clears the previous connection's entry")

	close(resume)
	<-published
	require.NoError(t, <-admitted)

	_, ok = f.served()
	assert.False(t, ok, "the publish computed for the previous connection must not be installed")
	responses := f.openListenerWatch(t, "")
	select {
	case r := <-responses:
		t.Fatalf("the new stream's first watch was answered with %q; it must wait for this connection's first publish", r.GetResponseVersion())
	case <-time.After(100 * time.Millisecond):
	}

	// The new connection's own publishes install, and answer that watch.
	f.translator.syncXds(ctx, f.build("v3"))
	select {
	case r := <-responses:
		assert.Equal(t, "v3", r.GetResponseVersion())
	case <-time.After(2 * time.Second):
		t.Fatal("the new connection's publish was not delivered")
	}
}

// Only a key's first stream starts a connection. A stream that joins a key
// with streams still connected is served the key's entry as it is, and
// publishes keep installing across the change of streams.
func TestStreamJoiningAConnectedKeyKeepsItsEntry(t *testing.T) {
	f := newConnectionStartFixture(t)

	f.connect(t, 1)
	f.translator.syncXds(t.Context(), f.build("v1"))
	f.connect(t, 2)
	version, ok := f.served()
	require.True(t, ok)
	assert.Equal(t, "v1", version, "a joining stream does not clear the key's entry")

	f.closeStream(1)
	f.translator.syncXds(context.Background(), f.build("v2"))
	version, ok = f.served()
	require.True(t, ok)
	assert.Equal(t, "v2", version, "publishes install while any stream of the key stays connected")
}

// A build that waited in the event queue while the client's row moved on must
// not be published. The connection-start hook publishes a returning client's
// current row as soon as its stream is admitted, ahead of the queue; the older
// build arriving afterwards would replace it on the new connection and answer
// the new stream's watch with the older config.
func TestDelayedBuildDoesNotRollBackANewConnection(t *testing.T) {
	f := newConnectionStartFixture(t)
	ctx := t.Context()
	f.translator.gate.currentBuild = func(string) *XdsSnapWrapper { return f.current.Load() }

	v1 := f.build("v1")
	f.current.Store(&v1)
	f.connect(t, 1)
	version, ok := f.served()
	require.True(t, ok)
	require.Equal(t, "v1", version)

	// The row moves on to v2 while v1's event is still queued, and the proxy
	// reconnects: the new connection is published the current row.
	v2 := f.build("v2")
	f.current.Store(&v2)
	f.closeStream(1)
	f.connect(t, 2)
	version, ok = f.served()
	require.True(t, ok)
	require.Equal(t, "v2", version)
	responses := f.openListenerWatch(t, "v2")

	// The queued v1 event finally reaches the gate.
	f.translator.syncXds(ctx, v1)
	version, _ = f.served()
	assert.Equal(t, "v2", version, "a superseded build must not replace the current one")
	select {
	case r := <-responses:
		t.Fatalf("the new stream's watch was answered with %q", r.GetResponseVersion())
	case <-time.After(100 * time.Millisecond):
	}

	// v2's own event publishes as usual.
	f.translator.syncXds(ctx, v2)
	version, _ = f.served()
	assert.Equal(t, "v2", version)
}

// Forgetting the previous connection and adopting the new generation happen in
// one critical section. A queued build that reaches the gate while a
// connection starts is decided either before the reset, which withdraws
// whatever it armed, or after the adoption, under the new generation. Never in
// between: a first-publish timer armed there would outlive the adoption and
// later publish the build to the new connection.
func TestNoBuildIsDecidedBetweenResetAndAdoption(t *testing.T) {
	f := newConnectionStartFixture(t)
	ctx := t.Context()
	g := f.translator.gate

	v1 := f.build("v1")
	f.current.Store(&v1)
	f.connect(t, 1)
	f.closeStream(1)

	queued := f.build("queued")
	queued.missingReferenced = []string{"cluster-not-ready"}
	f.current.Store(&queued)
	var mu sync.Mutex
	var decidedUnder []uint64
	g.currentBuild = func(key string) *XdsSnapWrapper {
		mu.Lock()
		decidedUnder = append(decidedUnder, g.connGen[key]) // the gate lock is held here
		mu.Unlock()
		return f.current.Load()
	}
	// While the connection starts, a queued build reaches the gate; give it
	// every chance to be decided before the adoption.
	g.connectionReset = func(string) {
		decided := make(chan struct{})
		go func() {
			defer close(decided)
			f.translator.syncXds(ctx, queued)
		}()
		select {
		case <-decided:
		case <-time.After(50 * time.Millisecond):
		}
	}
	f.connect(t, 2)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(decidedUnder) >= 2 // the queued build and the admission replay
	}, 2*time.Second, time.Millisecond)
	adopted := g.fence.current(f.role)
	mu.Lock()
	defer mu.Unlock()
	for _, gen := range decidedUnder {
		assert.Equal(t, adopted, gen, "a build was decided before the new connection's generation was adopted")
	}
}
