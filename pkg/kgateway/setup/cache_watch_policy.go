package setup

import (
	"context"
	"hash/maphash"
	"maps"
	"sync"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

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

// watchPolicy wraps the snapshot cache with three request rules.
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
// nothing. Cost: one EDS, RDS and SDS push per proxy that reconnects to an
// empty cache; a warm cache answers the request anyway.
//
// Re-warms: when a cluster response changes a cluster the stream already has,
// Envoy builds the new cluster and warms it on endpoints, keeping CDS paused
// until it is warm. It re-requests EDS at the version it holds, which the cache
// does not answer, so the cluster waits for its fetch timeout unless the
// endpoint version moves after the cluster response. Snapshot staging decides
// that order: an endpoint change published before or with the cluster change
// can reach Envoy first and update only the old cluster. So the policy notes
// which clusters each cluster response re-defined, and the stream's next
// endpoint watch treats them as not yet returned; the cache then answers with
// their assignments at the current version. An endpoint response sent after
// the cluster response ends the rule for each assignment it carries, and a
// cluster response ends it for clusters it no longer has. Always on. Cost: one
// hash per cluster per cluster response, and an endpoint push of the
// re-defined clusters only.
type watchPolicy struct {
	envoycache.SnapshotCache
	hasher envoycache.NodeHash
	seed   maphash.Seed

	// publish is held by SetSnapshot and ClearSnapshot, while a NACK is
	// parked (see CreateWatch), and by a retry from its version check through
	// replacing the parked watch (see retryLater).
	publish sync.Mutex

	suppressNackResend bool
	respondOnReconnect bool

	mu      sync.Mutex
	streams map[int64]*streamState
	pending map[*discoveryv3.DiscoveryRequest]pendingNack
	// rewarming associates an endpoint request with its stream while the stream
	// has re-defined clusters to answer; consumed like pending.
	rewarming map[*discoveryv3.DiscoveryRequest]*streamState
}

type (
	sentNackResponse struct{ node, nonce, version string }
	streamState      struct {
		sent    map[string]sentNackResponse
		pending *discoveryv3.DiscoveryRequest
		// rejections counts consecutive NACKs of one version per type, for the backoff.
		rejections map[string]nackRejections
		// clusters hashes each cluster's encoding in the last cluster response.
		clusters map[string]uint64
		// rewarm lists the endpoint resource names (EDS service_name, else the
		// cluster's name) of EDS clusters a cluster response re-defined that no
		// endpoint response has followed yet. Envoy requests endpoints, and the
		// subscription tracks them, by that name.
		rewarm map[string]struct{}
		// rewarmBy maps each re-defined cluster to its rewarm entry, so a
		// cluster that leaves takes its mark with it while another cluster
		// sharing the service name keeps it.
		rewarmBy map[string]string
		// rewarmRequest is this stream's entry in watchPolicy.rewarming.
		rewarmRequest *discoveryv3.DiscoveryRequest
	}
	nackRejections struct {
		version string
		count   int
	}
)

type pendingNack struct {
	stream   *streamState
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
		seed:               maphash.MakeSeed(),
		suppressNackResend: suppressNackResend,
		respondOnReconnect: respondOnReconnect,
		streams:            make(map[int64]*streamState),
		pending:            make(map[*discoveryv3.DiscoveryRequest]pendingNack),
		rewarming:          make(map[*discoveryv3.DiscoveryRequest]*streamState),
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
		st = &streamState{sent: make(map[string]sentNackResponse), rejections: make(map[string]nackRejections)}
		s.streams[streamID] = st
	}
	// The server may drop a stale-nonce request without calling CreateWatch, so
	// replace any earlier association now.
	delete(s.pending, st.pending)
	st.pending = nil
	delete(s.rewarming, st.rewarmRequest)
	st.rewarmRequest = nil
	if req.GetTypeUrl() == envoyresource.EndpointType && len(st.rewarm) > 0 {
		st.rewarmRequest = req
		s.rewarming[req] = st
	}
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
	st := s.streams[streamID]
	if st == nil {
		return
	}
	st.sent[resp.GetTypeUrl()] = sentNackResponse{node: s.hasher.ID(req.GetNode()), nonce: resp.GetNonce(), version: resp.GetVersionInfo()}
	// The server calls this before sending, on the goroutine that sends, so the
	// order seen here is the order on the wire.
	switch resp.GetTypeUrl() {
	case envoyresource.ClusterType:
		s.noteClustersLocked(st, resp.GetResources())
	case envoyresource.EndpointType:
		// Envoy applies this after the cluster response, so each assignment in
		// it reaches the re-defined cluster. Only those it carries are done: a
		// same-version response built before the cluster response went out
		// carries only newly subscribed names.
		for _, resource := range resp.GetResources() {
			if name, ok := resourceName(resource.GetValue()); ok {
				st.rewarmDone(name)
			}
		}
	}
}

// noteClustersLocked records the clusters in a cluster response and marks the
// EDS ones whose encoding changed since the stream's previous cluster response,
// under the name Envoy requests their endpoints by. The cache marshals
// deterministically, so equal clusters hash equal, and only a changed cluster
// is decoded. A cluster new to the stream is left alone: Envoy subscribes to
// its endpoints, and the cache answers new names itself. A cluster that takes
// no endpoint resources, or no longer does, is not marked. A removed cluster no
// longer warms, so its mark goes unless another re-defined cluster shares it.
func (s *watchPolicy) noteClustersLocked(st *streamState, resources []*anypb.Any) {
	sent := make(map[string]uint64, len(resources))
	for _, resource := range resources {
		name, ok := resourceName(resource.GetValue())
		if !ok {
			continue
		}
		hash := maphash.Bytes(s.seed, resource.GetValue())
		sent[name] = hash
		if previous, had := st.clusters[name]; !had || previous == hash {
			continue
		}
		endpoints, eds := endpointResourceName(resource.GetValue())
		if !eds {
			delete(st.rewarmBy, name)
			continue
		}
		if st.rewarm == nil {
			st.rewarm = make(map[string]struct{})
			st.rewarmBy = make(map[string]string)
		}
		st.rewarm[endpoints] = struct{}{}
		st.rewarmBy[name] = endpoints
	}
	for cluster := range st.rewarmBy {
		if _, kept := sent[cluster]; !kept {
			delete(st.rewarmBy, cluster)
		}
	}
	marked := make(map[string]struct{}, len(st.rewarmBy))
	for _, endpoints := range st.rewarmBy {
		marked[endpoints] = struct{}{}
	}
	for endpoints := range st.rewarm {
		if _, ok := marked[endpoints]; !ok {
			delete(st.rewarm, endpoints)
		}
	}
	st.clusters = sent
}

// rewarmDone ends the mark on an endpoint resource an endpoint response
// carried, for every cluster re-defined under it.
func (st *streamState) rewarmDone(endpoints string) {
	delete(st.rewarm, endpoints)
	for cluster, name := range st.rewarmBy {
		if name == endpoints {
			delete(st.rewarmBy, cluster)
		}
	}
}

// endpointResourceName decodes a cluster for the name Envoy requests its
// endpoints by: its EDS service_name, else its name. ok is false for a cluster
// that takes no endpoint resources.
func endpointResourceName(encoded []byte) (string, bool) {
	var cluster envoyclusterv3.Cluster
	if err := proto.Unmarshal(encoded, &cluster); err != nil || cluster.GetType() != envoyclusterv3.Cluster_EDS {
		return "", false
	}
	if name := cluster.GetEdsClusterConfig().GetServiceName(); name != "" {
		return name, true
	}
	return cluster.GetName(), true
}

// resourceName reads field 1 of an encoded Cluster (name) or
// ClusterLoadAssignment (cluster_name) without decoding the rest.
func resourceName(encoded []byte) (string, bool) {
	for len(encoded) > 0 {
		num, typ, n := protowire.ConsumeTag(encoded)
		if n < 0 {
			return "", false
		}
		encoded = encoded[n:]
		if num == 1 && typ == protowire.BytesType {
			name, n := protowire.ConsumeBytes(encoded)
			if n < 0 {
				return "", false
			}
			return string(name), true
		}
		n = protowire.ConsumeFieldValue(num, typ, encoded)
		if n < 0 {
			return "", false
		}
		encoded = encoded[n:]
	}
	return "", false
}

func (s *watchPolicy) onStreamClosed(streamID int64, _ *envoycorev3.Node) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.streams[streamID]; st != nil {
		delete(s.pending, st.pending)
		delete(s.rewarming, st.rewarmRequest)
	}
	delete(s.streams, streamID)
}

// rewarmSubscription returns sub with the stream's re-defined clusters treated
// as not yet returned, if req is an endpoint request on a stream that has some;
// otherwise sub itself. Matched by pointer like rejectedVersion.
func (s *watchPolicy) rewarmSubscription(req *envoycache.Request, sub envoycache.Subscription) envoycache.Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.rewarming[req]
	if !ok {
		return sub
	}
	delete(s.rewarming, req)
	st.rewarmRequest = nil
	if len(st.rewarm) == 0 {
		return sub
	}
	envoyLogger.Debug("answering an endpoint request with the clusters a cluster response re-defined",
		"node", s.hasher.ID(req.GetNode()), "clusters", len(st.rewarm), "held_version", req.GetVersionInfo())
	return unreturnedSubscription{Subscription: sub, unreturned: maps.Clone(st.rewarm)}
}

// unreturnedSubscription reports the unreturned resources as not yet sent, so
// the cache answers them even at the client's version.
type unreturnedSubscription struct {
	envoycache.Subscription
	unreturned map[string]struct{}
}

func (u unreturnedSubscription) ReturnedResources() map[string]string {
	returned := u.Subscription.ReturnedResources()
	out := make(map[string]string, len(returned))
	for name, version := range returned {
		if _, unreturned := u.unreturned[name]; !unreturned {
			out[name] = version
		}
	}
	return out
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

// CreateWatch applies the three rules and delegates everything else.
func (s *watchPolicy) CreateWatch(req *envoycache.Request, sub envoycache.Subscription, ch chan envoycache.Response) (func(), error) {
	sub = s.rewarmSubscription(req, sub)
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
