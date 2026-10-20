package setup

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"slices"
	"sync"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errBoom = errors.New("boom")

// hookCall is one hook a recordingCallbacks saw.
type hookCall struct {
	recorder string
	hook     string
	streamID int64
	typeURL  string
	version  string
	nonce    string
	nack     bool
}

// callLog records hook calls across recorders, in order. Server goroutines
// write it while tests read it.
type callLog struct {
	mu    sync.Mutex
	calls []hookCall
}

func (l *callLog) add(c hookCall) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, c)
}

func (l *callLog) all() []hookCall {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.calls)
}

func (l *callLog) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = nil
}

// find returns the first call that matches.
func (l *callLog) find(match func(hookCall) bool) (hookCall, bool) {
	for _, c := range l.all() {
		if match(c) {
			return c, true
		}
	}
	return hookCall{}, false
}

// recordingCallbacks records every hook and returns fail from each hook that
// can reject.
type recordingCallbacks struct {
	name string
	log  *callLog
	fail error
}

var _ xdsserver.Callbacks = (*recordingCallbacks)(nil)

func (r *recordingCallbacks) record(c hookCall) {
	c.recorder = r.name
	r.log.add(c)
}

func (r *recordingCallbacks) OnStreamOpen(_ context.Context, id int64, typeURL string) error {
	r.record(hookCall{hook: "OnStreamOpen", streamID: id, typeURL: typeURL})
	return r.fail
}

func (r *recordingCallbacks) OnStreamClosed(id int64, _ *envoycorev3.Node) {
	r.record(hookCall{hook: "OnStreamClosed", streamID: id})
}

func (r *recordingCallbacks) OnStreamRequest(id int64, req *discoveryv3.DiscoveryRequest) error {
	r.record(hookCall{
		hook: "OnStreamRequest", streamID: id, typeURL: req.GetTypeUrl(),
		version: req.GetVersionInfo(), nonce: req.GetResponseNonce(), nack: req.GetErrorDetail() != nil,
	})
	return r.fail
}

func (r *recordingCallbacks) OnStreamResponse(_ context.Context, id int64, _ *discoveryv3.DiscoveryRequest, resp *discoveryv3.DiscoveryResponse) {
	r.record(hookCall{
		hook: "OnStreamResponse", streamID: id, typeURL: resp.GetTypeUrl(),
		version: resp.GetVersionInfo(), nonce: resp.GetNonce(),
	})
}

func (r *recordingCallbacks) OnFetchRequest(context.Context, *discoveryv3.DiscoveryRequest) error {
	r.record(hookCall{hook: "OnFetchRequest"})
	return r.fail
}

func (r *recordingCallbacks) OnFetchResponse(*discoveryv3.DiscoveryRequest, *discoveryv3.DiscoveryResponse) {
	r.record(hookCall{hook: "OnFetchResponse"})
}

func (r *recordingCallbacks) OnDeltaStreamOpen(_ context.Context, id int64, typeURL string) error {
	r.record(hookCall{hook: "OnDeltaStreamOpen", streamID: id, typeURL: typeURL})
	return r.fail
}

func (r *recordingCallbacks) OnDeltaStreamClosed(id int64, _ *envoycorev3.Node) {
	r.record(hookCall{hook: "OnDeltaStreamClosed", streamID: id})
}

func (r *recordingCallbacks) OnStreamDeltaRequest(id int64, _ *discoveryv3.DeltaDiscoveryRequest) error {
	r.record(hookCall{hook: "OnStreamDeltaRequest", streamID: id})
	return r.fail
}

func (r *recordingCallbacks) OnStreamDeltaResponse(id int64, _ *discoveryv3.DeltaDiscoveryRequest, _ *discoveryv3.DeltaDiscoveryResponse) {
	r.record(hookCall{hook: "OnStreamDeltaResponse", streamID: id})
}

// everyHook calls each xdsserver.Callbacks method on cb, by name.
func everyHook(cb xdsserver.Callbacks) map[string]func() error {
	ctx := context.Background()
	node := &envoycorev3.Node{}
	req, resp := &discoveryv3.DiscoveryRequest{}, &discoveryv3.DiscoveryResponse{}
	deltaReq, deltaResp := &discoveryv3.DeltaDiscoveryRequest{}, &discoveryv3.DeltaDiscoveryResponse{}
	return map[string]func() error{
		"OnStreamOpen":          func() error { return cb.OnStreamOpen(ctx, 1, "") },
		"OnStreamClosed":        func() error { cb.OnStreamClosed(1, node); return nil },
		"OnStreamRequest":       func() error { return cb.OnStreamRequest(1, req) },
		"OnStreamResponse":      func() error { cb.OnStreamResponse(ctx, 1, req, resp); return nil },
		"OnFetchRequest":        func() error { return cb.OnFetchRequest(ctx, req) },
		"OnFetchResponse":       func() error { cb.OnFetchResponse(req, resp); return nil },
		"OnDeltaStreamOpen":     func() error { return cb.OnDeltaStreamOpen(ctx, 1, "") },
		"OnDeltaStreamClosed":   func() error { cb.OnDeltaStreamClosed(1, node); return nil },
		"OnStreamDeltaRequest":  func() error { return cb.OnStreamDeltaRequest(1, deltaReq) },
		"OnStreamDeltaResponse": func() error { cb.OnStreamDeltaResponse(1, deltaReq, deltaResp); return nil },
	}
}

// callbackMethods returns the xdsserver.Callbacks method names, split by
// whether the method can reject (returns an error).
func callbackMethods() (rejecting, notifying []string) {
	errorType := reflect.TypeFor[error]()
	iface := reflect.TypeFor[xdsserver.Callbacks]()
	for m := range iface.Methods() {
		m := m
		if m.Type.NumOut() == 1 && m.Type.Out(0) == errorType {
			rejecting = append(rejecting, m.Name)
		} else {
			notifying = append(notifying, m.Name)
		}
	}
	return rejecting, notifying
}

func hooksOf(calls []hookCall) []string {
	var out []string
	for _, c := range calls {
		out = append(out, c.recorder+":"+c.hook)
	}
	return out
}

// TestChainCallbacksDeliversEveryHook: each hook reaches every callback, in
// chain order. The table must cover every interface method, so a hook added by
// a go-control-plane bump fails here by name until it is covered.
func TestChainCallbacksDeliversEveryHook(t *testing.T) {
	log := &callLog{}
	chain := ChainCallbacks(&recordingCallbacks{name: "first", log: log}, &recordingCallbacks{name: "second", log: log})
	hooks := everyHook(chain)

	rejecting, notifying := callbackMethods()
	require.ElementsMatch(t, append(rejecting, notifying...), slices.Collect(maps.Keys(hooks)),
		"everyHook must call every xdsserver.Callbacks method")

	for name, call := range hooks {
		log.reset()
		require.NoError(t, call(), name)
		assert.Equal(t, []string{"first:" + name, "second:" + name}, hooksOf(log.all()), name)
	}
}

// TestChainCallbacksStopsAtFirstError: a hook that can reject stops at the
// first callback that does, and returns its error.
func TestChainCallbacksStopsAtFirstError(t *testing.T) {
	rejecting, _ := callbackMethods()
	require.Len(t, rejecting, 5)

	for _, name := range rejecting {
		t.Run(name, func(t *testing.T) {
			log := &callLog{}
			chain := ChainCallbacks(&recordingCallbacks{name: "first", log: log, fail: errBoom}, &recordingCallbacks{name: "second", log: log})

			require.ErrorIs(t, everyHook(chain)[name](), errBoom)
			assert.Equal(t, []string{"first:" + name}, hooksOf(log.all()), "the second callback must not see a rejected hook")
		})
	}
}

// TestChainCallbacksReachEveryCallbackAfterAnError: close and response hooks
// reach every callback even after one rejected the stream, as go-control-plane
// still closes a stream whose open failed.
func TestChainCallbacksReachEveryCallbackAfterAnError(t *testing.T) {
	_, notifying := callbackMethods()
	require.Len(t, notifying, 5)

	log := &callLog{}
	chain := ChainCallbacks(&recordingCallbacks{name: "first", log: log, fail: errBoom}, &recordingCallbacks{name: "second", log: log})
	hooks := everyHook(chain)
	require.ErrorIs(t, hooks["OnStreamOpen"](), errBoom)

	for _, name := range notifying {
		log.reset()
		require.NoError(t, hooks[name]())
		assert.Equal(t, []string{"first:" + name, "second:" + name}, hooksOf(log.all()), name)
	}
}

// TestChainCallbacksSkipsNil: an untyped nil and a nil pointer are both
// skipped, so an unset extra callback does not panic the server.
func TestChainCallbacksSkipsNil(t *testing.T) {
	log := &callLog{}
	chain := ChainCallbacks(nil, (*recordingCallbacks)(nil), &recordingCallbacks{name: "second", log: log})

	for name, call := range everyHook(chain) {
		log.reset()
		require.NotPanics(t, func() { _ = call() }, name)
		assert.Equal(t, []string{"second:" + name}, hooksOf(log.all()), name)
	}
}
