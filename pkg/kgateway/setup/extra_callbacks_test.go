package setup

import (
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/status"
)

// TestExtraCallbacksSeeEveryStreamHook: through go-control-plane's ADS server,
// callbacks chained ahead of kgateway's see the stream open, each response
// with the version and nonce sent, the NACK of it, and the close. A NACK
// detector needs exactly this; before, extra callbacks got only requests and
// the close.
func TestExtraCallbacksSeeEveryStreamHook(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(adsModeName(ordered), func(t *testing.T) {
			log := &callLog{}
			extra := &recordingCallbacks{name: "extra", log: log}
			// The empty CallbackFuncs stands in for the unique-client callbacks;
			// this test is about what reaches the extra ones.
			h := newADSHarnessWithCallbacks(t, ordered, ChainCallbacks(extra, xdsserver.CallbackFuncs{}))
			hook := func(name string) func(hookCall) bool {
				return func(c hookCall) bool { return c.hook == name }
			}

			var open hookCall
			require.Eventually(t, func() bool {
				var ok bool
				open, ok = log.find(hook("OnStreamOpen"))
				return ok
			}, 2*time.Second, 10*time.Millisecond)
			assert.Empty(t, open.typeURL, "an ADS stream opens with no type URL")

			h.setSnapshot(snapshotFor(allADSVersions("1"), []*envoyclusterv3.Cluster{testCluster(adsTestCluster)}, nil))
			h.subscribe(resource.ClusterType)
			resp := h.receiveType(resource.ClusterType, "1")

			// go-control-plane calls OnStreamResponse before Send, so it is
			// recorded by the time the response arrives.
			sent, ok := log.find(hook("OnStreamResponse"))
			require.True(t, ok, "the extra callbacks must see the response")
			assert.Equal(t, open.streamID, sent.streamID)
			assert.Equal(t, resp.GetVersionInfo(), sent.version)
			assert.Equal(t, resp.GetNonce(), sent.nonce)

			h.send(&discoveryv3.DiscoveryRequest{
				Node:          h.node,
				TypeUrl:       resource.ClusterType,
				ResponseNonce: resp.GetNonce(),
				ErrorDetail:   &status.Status{Message: "rejected by the test"},
			})
			require.Eventually(t, func() bool {
				_, ok := log.find(func(c hookCall) bool {
					return c.hook == "OnStreamRequest" && c.nack && c.nonce == resp.GetNonce()
				})
				return ok
			}, 2*time.Second, 10*time.Millisecond, "the extra callbacks must see the NACK of that response")

			// Don't read h.done here: the harness cleanup reads it.
			h.cancel()
			require.Eventually(t, func() bool {
				_, ok := log.find(func(c hookCall) bool {
					return c.hook == "OnStreamClosed" && c.streamID == open.streamID
				})
				return ok
			}, 2*time.Second, 10*time.Millisecond, "the extra callbacks must see the stream close")
		})
	}
}
