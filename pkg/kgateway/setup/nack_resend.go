package setup

import (
	"context"
	"sync"
	"time"

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
// Parking is not forever. A rejection can be transient (a secret or file the
// config names not there yet, a resource limit), and nothing else would offer
// the config again until an unrelated change. So a parked NACK is retried: once
// the retry delay passes with the snapshot unchanged, the rejected response is
// re-sent, and Envoy re-evaluates it. Consecutive rejections of one version on
// a stream double the delay, from nackRetryBase up to nackRetryMax, so a
// config that keeps failing costs one response per type per nackRetryMax
// rather than a spin. An ACK, or a rejection of a different version, starts
// over.
//
// Sent versions and nonces are tracked per stream and resource type. Nonces
// are only unique within a stream: two streams sharing a cache node can both
// use nonce "1" for different versions. A NACK may suppress only the version
// sent on its own stream with its matching nonce. Request associations are
// consumed by CreateWatch, replaced by the next request, or removed on close.
type nackResendSuppressor struct {
	envoycache.SnapshotCache
	hasher envoycache.NodeHash

	// publish is held by SetSnapshot and ClearSnapshot, while a NACK is
	// parked (see CreateWatch), and by a retry from its version check through
	// replacing the parked watch (see retryLater).
	publish sync.Mutex

	mu      sync.Mutex
	streams map[int64]*nackStreamState
	pending map[*discoveryv3.DiscoveryRequest]pendingNack
}

type (
	sentNackResponse struct{ node, nonce, version string }
	nackStreamState  struct {
		sent    map[string]sentNackResponse
		pending *discoveryv3.DiscoveryRequest
		// rejections counts consecutive NACKs of one version per type, for the backoff.
		rejections map[string]nackRejections
	}
	nackRejections struct {
		version string
		count   int
	}
)

type pendingNack struct {
	stream   *nackStreamState
	response sentNackResponse
	// retryIn is how long the parked NACK waits before its response is re-sent.
	retryIn time.Duration
}

// nackRetryBase and nackRetryMax bound the delay before a parked NACK's
// response is re-sent (see nackResendSuppressor). Variables so tests can
// shorten them.
var (
	nackRetryBase = time.Second
	nackRetryMax  = time.Minute
)

// nackRetryDelay is the delay after n consecutive rejections: nackRetryBase
// doubled n-1 times, capped at nackRetryMax.
func nackRetryDelay(n int) time.Duration {
	delay := nackRetryBase
	for i := 1; i < n && delay < nackRetryMax; i++ {
		delay *= 2
	}
	return min(delay, nackRetryMax)
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
		st = &nackStreamState{sent: make(map[string]sentNackResponse), rejections: make(map[string]nackRejections)}
		s.streams[streamID] = st
	}
	// The server may drop a stale-nonce request without calling CreateWatch, so
	// replace any earlier association now.
	delete(s.pending, st.pending)
	st.pending = nil
	if req.GetErrorDetail() == nil {
		// An ACK (or a fresh subscription) ends a run of rejections.
		delete(st.rejections, req.GetTypeUrl())
	}
	sent, ok := st.sent[req.GetTypeUrl()]
	if req.GetErrorDetail() != nil && ok && sent.nonce != "" && sent.nonce == req.GetResponseNonce() && sent.node == s.hasher.ID(req.GetNode()) {
		rejected := st.rejections[req.GetTypeUrl()]
		if rejected.version != sent.version {
			rejected = nackRejections{version: sent.version}
		}
		rejected.count++
		st.rejections[req.GetTypeUrl()] = rejected
		st.pending = req
		s.pending[req] = pendingNack{stream: st, response: sent, retryIn: nackRetryDelay(rejected.count)}
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
// the original request observed by logging/auth callbacks, and reports the
// rejected version and how long a parked NACK of it waits before its response
// is re-sent. The pinned SotW server passes that same request pointer to
// CreateWatch in both ADS modes. If that contract changes, an unassociated
// request passes through unchanged.
func (s *nackResendSuppressor) rejectedVersion(req *envoycache.Request) (string, time.Duration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.pending[req]
	if !ok {
		return "", 0, false
	}
	delete(s.pending, req)
	pending.stream.pending = nil
	return pending.response.version, pending.retryIn, true
}

// CreateWatch parks a NACK of the current snapshot version and delegates
// everything else.
func (s *nackResendSuppressor) CreateWatch(req *envoycache.Request, sub envoycache.Subscription, ch chan envoycache.Response) (func(), error) {
	if req.GetErrorDetail() == nil {
		return s.SnapshotCache.CreateWatch(req, sub, ch)
	}
	node := s.hasher.ID(req.GetNode())
	rejected, retryIn, tracked := s.rejectedVersion(req)
	if !tracked {
		return s.SnapshotCache.CreateWatch(req, sub, ch)
	}
	// Park under the publication lock, so the snapshot cannot move between the
	// version check and registering the parked watch: the cache drops a parked
	// request it cannot answer at a snapshot published in between. The stream
	// never waits on a publication, which may be sending to this stream, so
	// while one is in progress the NACK is handled as the plain cache would.
	if !s.publish.TryLock() {
		return s.SnapshotCache.CreateWatch(req, sub, ch)
	}
	defer s.publish.Unlock()
	snapshot, err := s.SnapshotCache.GetSnapshot(node)
	if err != nil {
		return s.SnapshotCache.CreateWatch(req, sub, ch)
	}
	current := snapshot.GetVersion(req.GetTypeUrl())
	if current == "" || current != rejected {
		// The snapshot moved and may carry a correction; let the cache answer.
		return s.SnapshotCache.CreateWatch(req, sub, ch)
	}
	// Claim the current version to park this watch. Retry the rejected config
	// after retryIn if no correction arrives.
	parked := proto.Clone(req).(*discoveryv3.DiscoveryRequest)
	parked.VersionInfo = current
	xdsNackResendsSuppressed.Inc(metrics.Label{Name: typeURLLabel, Value: rejectionTypeURL(req.GetTypeUrl())})
	envoyLogger.Debug("not re-sending a NACKed xDS response until the snapshot changes or the retry delay passes",
		"node", node, "type_url", req.GetTypeUrl(), "version", current, "retry_in", retryIn)
	cancel, err := s.SnapshotCache.CreateWatch(parked, sub, ch)
	if err != nil {
		return cancel, err
	}
	// The timer reads its own copy: later requests on the stream that omit Node
	// share req's Node, whose metadata the unique-client callback rewrites.
	retry := proto.Clone(req).(*discoveryv3.DiscoveryRequest)
	return s.retryLater(cancel, node, rejected, retry, sub, ch, retryIn), nil
}

// SetSnapshot publishes under the publication lock; see retryLater.
func (s *nackResendSuppressor) SetSnapshot(ctx context.Context, node string, snapshot envoycache.ResourceSnapshot) error {
	s.publish.Lock()
	defer s.publish.Unlock()
	return s.SnapshotCache.SetSnapshot(ctx, node, snapshot)
}

// ClearSnapshot clears under the publication lock; see retryLater.
func (s *nackResendSuppressor) ClearSnapshot(node string) {
	s.publish.Lock()
	defer s.publish.Unlock()
	s.SnapshotCache.ClearSnapshot(node)
}

// retryLater wraps a parked watch so that, every delay, it retries the rejected
// response: while the snapshot still holds the rejected version, it creates a
// watch for the original NACK, and only if the cache answers that watch at once
// does it replace the parked watch. Otherwise it keeps the parked watch and
// tries again after delay. The cache keeps a parked watch it cannot answer yet
// (an ADS named watch, while the snapshot holds resources outside its names)
// but drops a new one, so replacing the parked watch before a replacement is
// answered could leave the stream with no watch for that type. The snapshot
// can also move away from the rejected version and come back while the parked
// watch waits, so the retry keeps its schedule until the watch is cancelled.
//
// The retry runs on a timer goroutine. It holds the publication lock, so no
// snapshot is published between its version check and the replacement, and the
// watch's lock, because the server modifies the subscription the cache reads
// once cancel returns, and cancel takes that lock. Nothing under these locks
// blocks: the replacement answers into a channel of its own, and a lock-free
// forwarder hands the response on until the watch is cancelled. The server
// never waits on the publication lock (CreateWatch only tries it), so a
// publication blocked on a stream's channel cannot wait on the retry.
func (s *nackResendSuppressor) retryLater(parked func(), node, rejected string, original *envoycache.Request, sub envoycache.Subscription, ch chan envoycache.Response, delay time.Duration) func() {
	w := &retryingWatch{inner: parked, done: make(chan struct{})}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.timer = time.AfterFunc(delay, func() {
		s.publish.Lock()
		defer s.publish.Unlock()
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.cancelled {
			return
		}
		if snapshot, err := s.SnapshotCache.GetSnapshot(node); err != nil || snapshot.GetVersion(original.GetTypeUrl()) != rejected {
			w.timer.Reset(delay)
			return
		}
		own := make(chan envoycache.Response, 1)
		replacement, err := s.SnapshotCache.CreateWatch(original, sub, own)
		if err != nil {
			envoyLogger.Error("failed to re-send a NACKed xDS response", "type_url", original.GetTypeUrl(), "error", err)
			w.timer.Reset(delay)
			return
		}
		var resp envoycache.Response
		select {
		case resp = <-own:
		default:
			replacement()
			w.timer.Reset(delay)
			return
		}
		w.inner() // the cache must not also answer the parked watch
		w.inner = replacement
		go func() {
			select {
			case ch <- resp:
			case <-w.done:
			}
		}()
	})
	return w.cancel
}

// retryingWatch is a parked NACK watch with its pending retry.
type retryingWatch struct {
	timer *time.Timer
	done  chan struct{}

	mu        sync.Mutex
	inner     func()
	cancelled bool
}

func (w *retryingWatch) cancel() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancelled {
		return
	}
	w.cancelled = true
	w.timer.Stop()
	if w.inner != nil {
		w.inner()
		w.inner = nil
	}
	close(w.done)
}
