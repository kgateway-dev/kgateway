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
// when the snapshot's current version for the type equals the version this
// node was last sent for it, and parks the watch instead: the request handed
// to the cache claims the current version, so the cache registers a watch that
// fires on the next snapshot change and sends nothing now. A NACK arriving
// after the snapshot already moved is passed through unchanged, so a
// correction is delivered immediately. ACKs and first requests are never
// touched.
//
// Versions are tracked per cache node and type from the server's response
// callback. Several streams can share a node key; that is safe because a NACK
// of a version is the same decision for every stream that holds it, and a
// stream that has not been sent that version cannot NACK it.
type nackResendSuppressor struct {
	envoycache.SnapshotCache
	hasher envoycache.NodeHash

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

func newNackResendSuppressor(inner envoycache.SnapshotCache, hasher envoycache.NodeHash) *nackResendSuppressor {
	return &nackResendSuppressor{
		SnapshotCache: inner,
		hasher:        hasher,
		lastSent:      make(map[sentVersionKey]string),
		streamNode:    make(map[int64]string),
		nodeStreams:   make(map[string]int),
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
	node := s.hasher.ID(req.GetNode())
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, known := s.streamNode[streamID]; !known {
		s.streamNode[streamID] = node
		s.nodeStreams[node]++
	}
	return nil
}

func (s *nackResendSuppressor) onStreamResponse(_ context.Context, _ int64, req *discoveryv3.DiscoveryRequest, resp *discoveryv3.DiscoveryResponse) {
	key := sentVersionKey{node: s.hasher.ID(req.GetNode()), typeURL: resp.GetTypeUrl()}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSent[key] = resp.GetVersionInfo()
}

func (s *nackResendSuppressor) onStreamClosed(streamID int64, _ *envoycorev3.Node) {
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

// CreateWatch parks a NACK of the current snapshot version and delegates
// everything else.
func (s *nackResendSuppressor) CreateWatch(req *envoycache.Request, sub envoycache.Subscription, ch chan envoycache.Response) (func(), error) {
	if req.GetErrorDetail() == nil {
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
