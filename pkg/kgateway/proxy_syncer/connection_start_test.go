package proxy_syncer

import (
	"testing"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoydiscoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// A proxy that reconnects is never answered from the entry published to its
// previous connection: that entry can predate config it received meanwhile from
// another controller replica. The entry is cleared before the new stream is
// answered, and the client's current build is published to the new connection;
// with no current build the stream waits for the gate's first publish.
func TestReconnectIsNotServedThePreviousConnectionsEntry(t *testing.T) {
	// Read once per process; at worst each new stream below waits the 1s default.
	t.Setenv("KGW_XDS_FIRST_CONNECT_DELAY", "0s")
	ctx := t.Context()
	cb, uccBuilder, state := krtcollections.NewUniquelyConnectedClients(false)
	uccs := uccBuilder(ctx, krtutil.KrtOptions{}, nil)
	uccs.WaitUntilSynced(ctx.Done())

	role := wellknown.GatewayApiProxyValue + "~ns~gw"
	cache := newTestSnapshotCache(t)
	translator := NewProxyTranslator(cache, state, time.Hour, true)
	var current *XdsSnapWrapper
	require.True(t, translator.registerConnectionStartHooks(ctx, state, func(string) *XdsSnapWrapper { return current }))
	build := func(version string) XdsSnapWrapper {
		snap := &envoycache.Snapshot{}
		snap.Resources[envoycachetypes.Listener] = envoycache.NewResources(version, nil)
		return XdsSnapWrapper{snap: snap, proxyKey: role}
	}
	connect := func(sid int64) {
		t.Helper()
		require.NoError(t, cb.OnStreamRequest(sid, &envoydiscoveryv3.DiscoveryRequest{
			Node: &envoycorev3.Node{Id: "podname.ns", Metadata: &structpb.Struct{Fields: map[string]*structpb.Value{
				xds.RoleKey: structpb.NewStringValue(role),
			}}},
			TypeUrl: resourcev3.ListenerType,
		}))
	}
	served := func() (string, bool) {
		snap, err := cache.GetSnapshot(role)
		if err != nil {
			return "", false
		}
		return snap.GetVersion(resourcev3.ListenerType), true
	}

	connect(1)
	translator.syncXds(ctx, build("v1"))
	version, ok := served()
	require.True(t, ok)
	require.Equal(t, "v1", version)
	cb.OnStreamClosed(1, nil)

	// The proxy returns; this replica's current build for it is v2.
	v2 := build("v2")
	current = &v2
	connect(2)
	version, ok = served()
	require.True(t, ok)
	assert.Equal(t, "v2", version, "the new connection is answered from the current build, never the previous connection's entry")

	// Returning when there is no current build: the old entry is gone, so the
	// stream waits for the first publish instead of being answered from it.
	cb.OnStreamClosed(2, nil)
	current = nil
	connect(3)
	_, ok = served()
	assert.False(t, ok, "the previous connection's entry must not be served")
}
