package setup

import (
	"context"
	"sync"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

var xdsNackResendsSuppressed = metrics.NewCounter(
	metrics.CounterOpts{
		Subsystem: envoyXdsSubsystem,
		Name:      "nack_resends_suppressed_total",
		Help:      "Number of times a NACKed xDS response was not re-sent because the snapshot had not changed",
	}, []string{typeURLLabel})

// nackResendSuppressor wraps the snapshot cache so a NACK does not re-send the
// response that was just rejected.
//
// An xDS NACK is a DiscoveryRequest that carries the client's last accepted
// version and an error detail. The snapshot cache answers any request whose
// version differs from the snapshot's, and after a rejection those always
// differ: the client is still on the version it accepted before, the snapshot
// is the one it rejected. The cache therefore re-sends the rejected response,
// Envoy rejects it again immediately, and the two sides spin until the snapshot
// changes. Measured against a real proxy this runs in the thousands of
// responses per second per rejected type, all of it wasted, and it drowns the
// one line that says what was rejected.
//
// The suppressor recognizes a NACK of exactly what would be re-sent, which is
// when the snapshot's current version for the type equals the version identified
// by this stream's matching nonce, and parks the watch instead: the request handed
// to the cache claims the current version, so the cache registers a watch that
// fires on the next snapshot change and sends nothing now. A NACK arriving
// after the snapshot already moved is passed through unchanged, so a
// correction is delivered immediately. ACKs and first requests are never
// touched.
//
// Sent versions and nonces are tracked per stream and resource type. Nonces
// are only unique within a stream: two streams sharing a cache node can both
// use nonce "1" for different versions. A NACK may suppress only the version
// sent on its own stream with its matching nonce. Request associations are
// consumed by CreateWatch, replaced by the next request, or removed on close.
type nackResendSuppressor struct {
	envoycache.SnapshotCache
	hasher envoycache.NodeHash

	mu      sync.Mutex
	streams map[int64]*nackStreamState
	pending map[*discoveryv3.DiscoveryRequest]pendingNack
}

type (
	sentNackResponse struct{ node, nonce, version string }
	nackStreamState  struct {
		sent    map[string]sentNackResponse
		pending *discoveryv3.DiscoveryRequest
	}
)

type pendingNack struct {
	stream   *nackStreamState
	response sentNackResponse
}

func newNackResendSuppressor(inner envoycache.SnapshotCache, hasher envoycache.NodeHash) *nackResendSuppressor {
	return &nackResendSuppressor{
		SnapshotCache: inner,
		hasher:        hasher,
		streams:       make(map[int64]*nackStreamState),
		pending:       make(map[*discoveryv3.DiscoveryRequest]pendingNack),
	}
}

// callbacks returns the server callbacks that keep the version table current.
// They must be chained into the server that uses this cache.
func (s *nackResendSuppressor) callbacks() xdsserver.Callbacks {
	return xdsserver.CallbackFuncs{
		StreamRequestFunc:  s.onStreamRequest,
		StreamResponseFunc: s.onStreamResponse,
		StreamClosedFunc:   s.onStreamClosed,
	}
}

func (s *nackResendSuppressor) onStreamRequest(streamID int64, req *discoveryv3.DiscoveryRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.streams[streamID]
	if st == nil {
		st = &nackStreamState{sent: make(map[string]sentNackResponse)}
		s.streams[streamID] = st
	}
	// A callback can be followed by nonce rejection instead of CreateWatch.
	// Retain at most one association per stream, including on ignored requests.
	delete(s.pending, st.pending)
	st.pending = nil
	sent, ok := st.sent[req.GetTypeUrl()]
	if req.GetErrorDetail() != nil && ok && sent.nonce != "" && sent.nonce == req.GetResponseNonce() && sent.node == s.hasher.ID(req.GetNode()) {
		st.pending = req
		s.pending[req] = pendingNack{stream: st, response: sent}
	}
	return nil
}

func (s *nackResendSuppressor) onStreamResponse(_ context.Context, streamID int64, req *discoveryv3.DiscoveryRequest, resp *discoveryv3.DiscoveryResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.streams[streamID]; st != nil {
		st.sent[resp.GetTypeUrl()] = sentNackResponse{node: s.hasher.ID(req.GetNode()), nonce: resp.GetNonce(), version: resp.GetVersionInfo()}
	}
}

func (s *nackResendSuppressor) onStreamClosed(streamID int64, _ *envoycorev3.Node) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.streams[streamID]; st != nil {
		delete(s.pending, st.pending)
	}
	delete(s.streams, streamID)
}

// rejectedVersion bridges the stream callback to the cache without changing
// the original request observed by logging/auth callbacks. The pinned SotW
// server passes that same request pointer to CreateWatch in both ADS modes.
// If that contract changes, an unassociated request passes through unchanged.
func (s *nackResendSuppressor) rejectedVersion(req *envoycache.Request) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.pending[req]
	if !ok {
		return "", false
	}
	delete(s.pending, req)
	pending.stream.pending = nil
	return pending.response.version, true
}

// CreateWatch parks a NACK of the current snapshot version and delegates
// everything else.
func (s *nackResendSuppressor) CreateWatch(req *envoycache.Request, sub envoycache.Subscription, ch chan envoycache.Response) (func(), error) {
	if req.GetErrorDetail() == nil {
		return s.SnapshotCache.CreateWatch(req, sub, ch)
	}
	node := s.hasher.ID(req.GetNode())
	rejected, tracked := s.rejectedVersion(req)
	if !tracked {
		return s.SnapshotCache.CreateWatch(req, sub, ch)
	}
	snapshot, err := s.SnapshotCache.GetSnapshot(node)
	if err != nil {
		return s.SnapshotCache.CreateWatch(req, sub, ch)
	}
	current := snapshot.GetVersion(req.GetTypeUrl())
	if current == "" || current != rejected {
		// The snapshot moved since the rejected response: a correction may be
		// waiting, let the cache answer.
		return s.SnapshotCache.CreateWatch(req, sub, ch)
	}
	// Re-sending would deliver the rejected bytes again. Claim the current
	// version so the cache parks the watch until the snapshot changes.
	parked := proto.Clone(req).(*discoveryv3.DiscoveryRequest)
	parked.VersionInfo = current
	xdsNackResendsSuppressed.Inc(metrics.Label{Name: typeURLLabel, Value: req.GetTypeUrl()})
	envoyLogger.Debug("not re-sending a NACKed xDS response until the snapshot changes",
		"node", node, "type_url", req.GetTypeUrl(), "version", current)
	return s.SnapshotCache.CreateWatch(parked, sub, ch)
}
