package setup

import (
	"context"
	"sync"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

var (
	xdsNackResendsSuppressed = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: envoyXdsSubsystem,
			Name:      "nack_resends_suppressed_total",
			Help:      "Number of times a NACKed xDS response was not re-sent because the snapshot had not changed",
		}, []string{typeURLLabel})

	xdsReconnectResponses = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: envoyXdsSubsystem,
			Name:      "reconnect_responses_total",
			Help:      "Number of reconnecting proxies' first endpoint, route or secret requests whose held version was cleared, so the current or next snapshot answers them even at that version",
		}, []string{typeURLLabel})
)

// watchPolicy wraps the snapshot cache with two request rules.
//
// NACKs: a NACK carries the client's old version, which differs from the
// snapshot's, so the cache re-sends the rejected response and both sides spin
// (thousands of responses per second per type, burying the rejection log). When
// the version this stream sent under the NACKed nonce is still current, the
// policy parks the watch by claiming that version; other NACKs pass through, so
// a correction goes out at once. Rejections can be transient (a secret or file
// not there yet), so a parked NACK is re-sent after nackRetryBase, doubling per
// repeated rejection up to nackRetryMax: at most one response per type per
// nackRetryMax. An ACK or a different rejected version resets the delay. Nonces
// are unique only within a stream, so sent versions are tracked per stream and
// type. Request associations are consumed by CreateWatch, replaced by the next
// request, or removed on close.
//
// Reconnects: a proxy's first request for a type on a new stream carries its
// held version and no nonce. A warm cache answers it (nothing was returned on
// the stream yet). An empty cache, after a controller restart or reclaim, parks
// it, and an equal-version republish does not answer it (SetSnapshot compares
// only versions). For EDS, RDS and SDS the policy clears the version on a copy
// so the current or next snapshot answers; otherwise a cluster or listener
// warming on them waits for its fetch timeout, with CDS paused. CDS and LDS are
// left alone: nothing warms on them, and a parked watch at a held version costs
// nothing. Cost: one EDS, RDS and SDS push per reconnect.
type watchPolicy struct {
	envoycache.SnapshotCache
	hasher envoycache.NodeHash

	// publish is held by SetSnapshot and ClearSnapshot, while a NACK is
	// parked (see CreateWatch), and by a retry from its version check through
	// replacing the parked watch (see retryLater).
	publish sync.Mutex

	suppressNackResend bool
	respondOnReconnect bool

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

// nackRetryBase and nackRetryMax bound the parked-NACK retry delay (see
// watchPolicy). Variables so tests can shorten them.
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

func newWatchPolicy(inner envoycache.SnapshotCache, hasher envoycache.NodeHash, suppressNackResend, respondOnReconnect bool) *watchPolicy {
	return &watchPolicy{
		SnapshotCache:      inner,
		hasher:             hasher,
		suppressNackResend: suppressNackResend,
		respondOnReconnect: respondOnReconnect,
		streams:            make(map[int64]*nackStreamState),
		pending:            make(map[*discoveryv3.DiscoveryRequest]pendingNack),
	}
}

// callbacks tracks sent versions; chain it into the server using this cache.
func (s *watchPolicy) callbacks() xdsserver.Callbacks {
	return xdsserver.CallbackFuncs{
		StreamRequestFunc:  s.onStreamRequest,
		StreamResponseFunc: s.onStreamResponse,
		StreamClosedFunc:   s.onStreamClosed,
	}
}

func (s *watchPolicy) onStreamRequest(streamID int64, req *discoveryv3.DiscoveryRequest) error {
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

func (s *watchPolicy) onStreamResponse(_ context.Context, streamID int64, req *discoveryv3.DiscoveryRequest, resp *discoveryv3.DiscoveryResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.streams[streamID]; st != nil {
		st.sent[resp.GetTypeUrl()] = sentNackResponse{node: s.hasher.ID(req.GetNode()), nonce: resp.GetNonce(), version: resp.GetVersionInfo()}
	}
}

func (s *watchPolicy) onStreamClosed(streamID int64, _ *envoycorev3.Node) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.streams[streamID]; st != nil {
		delete(s.pending, st.pending)
	}
	delete(s.streams, streamID)
}

// rejectedVersion returns the rejected version and retry delay recorded for req,
// matched by pointer so the request logging/auth callbacks see is unchanged. The
// pinned SotW server passes that pointer in both ADS modes; an unassociated
// request passes through.
func (s *watchPolicy) rejectedVersion(req *envoycache.Request) (string, time.Duration, bool) {
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

// CreateWatch applies the two rules and delegates everything else.
func (s *watchPolicy) CreateWatch(req *envoycache.Request, sub envoycache.Subscription, ch chan envoycache.Response) (func(), error) {
	if req.GetErrorDetail() == nil {
		if s.respondOnReconnect && isReconnectRequest(req) && warmedOnTypes[req.GetTypeUrl()] {
			// Clear the held version so the current or next snapshot answers.
			fresh := proto.Clone(req).(*discoveryv3.DiscoveryRequest)
			fresh.VersionInfo = ""
			xdsReconnectResponses.Inc(metrics.Label{Name: typeURLLabel, Value: rejectionTypeURL(req.GetTypeUrl())})
			envoyLogger.Debug("answering a reconnecting proxy's first request regardless of its held version",
				"node", s.hasher.ID(req.GetNode()), "type_url", req.GetTypeUrl(), "held_version", req.GetVersionInfo())
			return s.SnapshotCache.CreateWatch(fresh, sub, ch)
		}
		return s.SnapshotCache.CreateWatch(req, sub, ch)
	}
	if !s.suppressNackResend {
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
func (s *watchPolicy) SetSnapshot(ctx context.Context, node string, snapshot envoycache.ResourceSnapshot) error {
	s.publish.Lock()
	defer s.publish.Unlock()
	return s.SnapshotCache.SetSnapshot(ctx, node, snapshot)
}

// ClearSnapshot clears under the publication lock; see retryLater.
func (s *watchPolicy) ClearSnapshot(node string) {
	s.publish.Lock()
	defer s.publish.Unlock()
	s.SnapshotCache.ClearSnapshot(node)
}

// retryLater re-sends the rejected response every delay while the snapshot
// still holds the rejected version, by creating a watch for the original NACK;
// it replaces the parked watch only if the cache answers that watch at once.
// The cache keeps a parked watch it cannot answer yet (an ADS named watch,
// while the snapshot holds resources outside its names) but drops a new one,
// and the snapshot can leave the rejected version and return, so the retry
// keeps the parked watch and its schedule until the watch is cancelled.
//
// The retry runs on a timer goroutine. It holds the publication lock from its
// version check through the replacement, so nothing is published in between,
// and the watch lock, because the server modifies the subscription the cache
// reads once cancel returns. Nothing under these locks blocks: the replacement
// answers into its own channel and a lock-free forwarder passes the response
// on until cancelled. The server never waits on the publication lock
// (CreateWatch only tries it).
func (s *watchPolicy) retryLater(parked func(), node, rejected string, original *envoycache.Request, sub envoycache.Subscription, ch chan envoycache.Response, delay time.Duration) func() {
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

// warmedOnTypes lists dependencies that can delay warming: EDS for clusters,
// RDS for listeners and SDS for either.
var warmedOnTypes = map[string]bool{
	envoyresource.EndpointType: true,
	envoyresource.RouteType:    true,
	envoyresource.SecretType:   true,
}

// isReconnectRequest reports a proxy's first request for a type on a new stream
// while it holds that type: a held version and no response nonce. Later requests
// for the type (ACK, NACK, subscription change) carry a nonce.
func isReconnectRequest(req *discoveryv3.DiscoveryRequest) bool {
	return req.GetResponseNonce() == "" && req.GetVersionInfo() != ""
}
