package setup

import (
	"context"
	"sync"

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
			Help:      "Number of times a reconnecting proxy's first endpoint request was answered although it already held the current version",
		}, []string{typeURLLabel})
)

// watchPolicy wraps the snapshot cache with two request-level rules the cache
// itself does not have: it does not re-send a response the proxy has just
// rejected, and it answers a reconnecting proxy's first endpoint request even
// when the proxy already holds the current version.
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
// The policy recognizes a NACK of exactly what would be re-sent, which is when
// the snapshot's current version for the type equals the version this node was
// last sent for it, and parks the watch instead: the request handed to the
// cache claims the current version, so the cache registers a watch that fires
// on the next snapshot change and sends nothing now. A NACK arriving after the
// snapshot already moved is passed through unchanged, so a correction is
// delivered immediately. ACKs and first requests are never touched by this
// rule.
//
// Versions are tracked per cache node and type from the server's response
// callback. Several streams can share a node key; that is safe because a NACK
// of a version is the same decision for every stream that holds it, and a
// stream that has not been sent that version cannot NACK it.
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
// equal, and sends nothing. For a cluster that was warming when the
// stream broke that is not harmless: Envoy rebuilds the cluster, asks for its
// endpoints again at the version it holds, gets no answer, and keeps CDS
// paused until its own endpoint fetch timeout releases it. The policy hands
// the cache a copy of the endpoint request with the version cleared, so the
// current snapshot answers it now or the next published one answers it then.
// Only the endpoint type is treated this way: a warming cluster waits on
// endpoints, a parked cluster or listener watch on a reconnect costs nothing,
// and named route and secret requests on a fresh stream are answered by the
// cache's own not-yet-returned rule. The cost is one endpoint push per
// reconnecting proxy.
type watchPolicy struct {
	envoycache.SnapshotCache
	hasher envoycache.NodeHash

	suppressNackResend bool
	respondOnReconnect bool

	mu sync.Mutex
	// lastSent is the version most recently sent to a node for a type.
	lastSent map[sentVersionKey]string
	// streamNode remembers each open stream's node so lastSent can be dropped
	// once no stream for that node remains.
	streamNode  map[int64]string
	nodeStreams map[string]int
}

type sentVersionKey struct {
	node    string
	typeURL string
}

func newWatchPolicy(inner envoycache.SnapshotCache, hasher envoycache.NodeHash, suppressNackResend, respondOnReconnect bool) *watchPolicy {
	return &watchPolicy{
		SnapshotCache:      inner,
		hasher:             hasher,
		suppressNackResend: suppressNackResend,
		respondOnReconnect: respondOnReconnect,
		lastSent:           make(map[sentVersionKey]string),
		streamNode:         make(map[int64]string),
		nodeStreams:        make(map[string]int),
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
	node := s.hasher.ID(req.GetNode())
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, known := s.streamNode[streamID]; !known {
		s.streamNode[streamID] = node
		s.nodeStreams[node]++
	}
	return nil
}

func (s *watchPolicy) onStreamResponse(_ context.Context, _ int64, req *discoveryv3.DiscoveryRequest, resp *discoveryv3.DiscoveryResponse) {
	key := sentVersionKey{node: s.hasher.ID(req.GetNode()), typeURL: resp.GetTypeUrl()}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSent[key] = resp.GetVersionInfo()
}

func (s *watchPolicy) onStreamClosed(streamID int64, _ *envoycorev3.Node) {
	s.mu.Lock()
	defer s.mu.Unlock()
	node, known := s.streamNode[streamID]
	if !known {
		return
	}
	delete(s.streamNode, streamID)
	s.nodeStreams[node]--
	if s.nodeStreams[node] > 0 {
		return
	}
	delete(s.nodeStreams, node)
	for key := range s.lastSent {
		if key.node == node {
			delete(s.lastSent, key)
		}
	}
}

// CreateWatch applies the two rules and delegates everything else.
func (s *watchPolicy) CreateWatch(req *envoycache.Request, sub envoycache.Subscription, ch chan envoycache.Response) (func(), error) {
	if req.GetErrorDetail() == nil {
		if s.respondOnReconnect && isReconnectRequest(req) && req.GetTypeUrl() == envoyresource.EndpointType {
			// Clear the held version so the current or next snapshot answers.
			fresh := proto.Clone(req).(*discoveryv3.DiscoveryRequest)
			fresh.VersionInfo = ""
			xdsReconnectResponses.Inc(metrics.Label{Name: typeURLLabel, Value: req.GetTypeUrl()})
			envoyLogger.Debug("answering a reconnecting proxy's first endpoint request regardless of its held version",
				"node", s.hasher.ID(req.GetNode()), "held_version", req.GetVersionInfo())
			return s.SnapshotCache.CreateWatch(fresh, sub, ch)
		}
		return s.SnapshotCache.CreateWatch(req, sub, ch)
	}
	if !s.suppressNackResend {
		return s.SnapshotCache.CreateWatch(req, sub, ch)
	}
	node := s.hasher.ID(req.GetNode())
	s.mu.Lock()
	rejected, tracked := s.lastSent[sentVersionKey{node: node, typeURL: req.GetTypeUrl()}]
	s.mu.Unlock()
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

// isReconnectRequest reports whether req is the first request on a stream from
// a proxy that already holds configuration: it names the version the proxy
// accepted on its previous stream and carries no response nonce, because no
// response has been sent on this stream yet. Every later request on the stream,
// ACK, NACK or subscription change, carries the nonce of the last response.
func isReconnectRequest(req *discoveryv3.DiscoveryRequest) bool {
	return req.GetResponseNonce() == "" && req.GetVersionInfo() != ""
}
