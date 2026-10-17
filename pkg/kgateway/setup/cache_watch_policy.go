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

// watchPolicy wraps the snapshot cache with two request-level rules the cache
// itself does not have: it does not re-send a response the proxy has just
// rejected, and it answers a reconnecting proxy's first endpoint, route and
// secret requests even when the proxy already holds the current version.
//
// Rule 1, NACKs.
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
// The policy recognizes a NACK of exactly what would be re-sent, which is
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
//
// Rule 2, reconnects.
//
// The first request on a new stream from a proxy that already holds
// configuration carries the version it accepted on its previous stream and no
// nonce. Against a warm cache the pinned cache answers it by itself, because
// nothing has been returned on the new stream yet. Against an empty cache,
// which is what a proxy meets when the control plane has just restarted, the
// request parks; and when the control plane then republishes the same content
// under the same version, the publish path compares versions only, finds them
// equal, and sends nothing. The same happens after the controller reclaims a
// departed proxy's entry and the proxy comes back. For a resource that was
// warming when the stream broke that is not harmless: a cluster waits on its
// endpoints (and Envoy keeps CDS paused meanwhile), a listener on its route
// configuration, and either on its secrets. Each asks again at the version it
// holds, gets no answer, and stays warming until its own fetch timeout
// releases it. The policy hands the cache a copy of such a request with the
// version cleared, so the current snapshot answers it now or the next
// published one answers it then. The cache's own not-yet-returned rule does
// not cover this: it answers a fresh stream's named request only when a
// snapshot already exists as the request arrives. Cluster and listener
// requests are left alone, since nothing warms on them and a parked watch for
// a version the proxy holds costs nothing. The cost is one endpoint, route and
// secret push per reconnecting proxy.
type watchPolicy struct {
	envoycache.SnapshotCache
	hasher envoycache.NodeHash

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
		// rejections counts, per type, consecutive NACKs of one version on
		// this stream, for the retry backoff.
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
	// retryIn is how long the parked NACK waits before its response is
	// re-sent with the snapshot unchanged.
	retryIn time.Duration
}

// nackRetryBase and nackRetryMax bound the delay before a parked NACK's
// response is re-sent (see watchPolicy). Variables so tests can
// shorten them.
var (
	nackRetryBase = time.Second
	nackRetryMax  = time.Minute
)

// nackRetryDelay is the delay before re-sending a response rejected n times in
// a row: nackRetryBase doubled per rejection after the first, at most
// nackRetryMax.
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

// callbacks returns the server callbacks that keep the version table current.
// They must be chained into the server that uses this cache.
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
	// A callback can be followed by nonce rejection instead of CreateWatch.
	// Retain at most one association per stream, including on ignored requests.
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

// rejectedVersion bridges the stream callback to the cache without changing
// the original request observed by logging/auth callbacks, and reports the
// rejected version and how long a parked NACK of it waits before its response
// is re-sent. The pinned SotW server passes that same request pointer to
// CreateWatch in both ADS modes. If that contract changes, an unassociated
// request passes through unchanged.
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
	// version so the cache parks the watch until the snapshot changes, and
	// re-send them only after retryIn.
	parked := proto.Clone(req).(*discoveryv3.DiscoveryRequest)
	parked.VersionInfo = current
	xdsNackResendsSuppressed.Inc(metrics.Label{Name: typeURLLabel, Value: rejectionTypeURL(req.GetTypeUrl())})
	envoyLogger.Debug("not re-sending a NACKed xDS response until the snapshot changes or the retry delay passes",
		"node", node, "type_url", req.GetTypeUrl(), "version", current, "retry_in", retryIn)
	cancel, err := s.SnapshotCache.CreateWatch(parked, sub, ch)
	if err != nil {
		return cancel, err
	}
	return s.retryLater(cancel, req, sub, ch, retryIn), nil
}

// retryLater wraps a parked watch so that, after delay, it is replaced by a
// watch for the original NACK, which the cache answers at once with the
// snapshot's current version: the rejected response again if nothing changed,
// a correction if the snapshot just moved.
//
// The replacement runs on a timer, beside the stream's own goroutine, so it is
// careful in two ways. The cache reads the stream's subscription while
// creating a watch, and the server modifies the subscription right after
// cancelling the watch; the replacement therefore holds the watch's lock while
// it creates one, and the returned cancel takes that lock, so it cannot return
// while the cache is still reading. And creating the watch must never block
// with that lock held, or the server, waiting in cancel, could not drain the
// stream's channel; the new watch therefore answers into a channel of its own,
// and a forwarder hands the response to the stream's channel holding no lock,
// giving up once the watch is cancelled.
func (s *watchPolicy) retryLater(parked func(), original *envoycache.Request, sub envoycache.Subscription, ch chan envoycache.Response, delay time.Duration) func() {
	w := &retryingWatch{inner: parked, done: make(chan struct{})}
	w.timer = time.AfterFunc(delay, func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.cancelled {
			return
		}
		if w.inner != nil {
			w.inner() // the cache must not also answer the parked watch
		}
		own := make(chan envoycache.Response, 1)
		inner, err := s.SnapshotCache.CreateWatch(original, sub, own)
		if err != nil {
			envoyLogger.Error("failed to re-send a NACKed xDS response", "type_url", original.GetTypeUrl(), "error", err)
			w.inner = nil
			return
		}
		w.inner = inner
		go func() {
			select {
			case resp := <-own:
				select {
				case ch <- resp:
				case <-w.done:
				}
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
	w.timer.Stop()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancelled {
		return
	}
	w.cancelled = true
	if w.inner != nil {
		w.inner()
		w.inner = nil
	}
	close(w.done)
}

// warmedOnTypes are the named types an Envoy resource waits on while it warms:
// a cluster on its endpoints, a listener on its route configuration, and
// either on its secrets.
var warmedOnTypes = map[string]bool{
	envoyresource.EndpointType: true,
	envoyresource.RouteType:    true,
	envoyresource.SecretType:   true,
}

// isReconnectRequest reports whether req is the first request on a stream from
// a proxy that already holds configuration: it names the version the proxy
// accepted on its previous stream and carries no response nonce, because no
// response has been sent on this stream yet. Every later request on the stream,
// ACK, NACK or subscription change, carries the nonce of the last response.
func isReconnectRequest(req *discoveryv3.DiscoveryRequest) bool {
	return req.GetResponseNonce() == "" && req.GetVersionInfo() != ""
}
