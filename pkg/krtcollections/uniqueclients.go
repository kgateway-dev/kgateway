package krtcollections

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"google.golang.org/protobuf/types/known/structpb"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/security"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// xdsFirstConnectDelay is slept on the first DiscoveryRequest of every new
// xDS stream, after the client has been registered (which kicks off
// per-client translation) and before returning to go-control-plane, which
// only then creates the stream's first watch. This gives the per-client
// cluster and endpoint collections a head start so the first snapshot the
// client observes is (almost always) fully converged rather than partially
// translated — the reconnect-time race that #13868's whole-snapshot
// readiness gates tried to close before they were reverted for causing
// indefinite starvation (#14184). Each gRPC stream runs on its own
// goroutine, so the sleep delays only this client and holds no locks; a
// reconnecting warm Envoy keeps serving its existing config meanwhile.
//
// Stored as nanoseconds in an atomic so the test override cannot race the
// stream goroutines that read it, and initialized lazily on first use so
// test binaries can set the environment variable from TestMain (package
// initialization would otherwise read the environment before any test code
// runs). Override with KGW_XDS_FIRST_CONNECT_DELAY (Go duration, e.g. "2s";
// "0" or negative disables).
var (
	xdsFirstConnectDelayNanos atomic.Int64
	xdsFirstConnectDelayInit  sync.Once
)

func xdsFirstConnectDelay() time.Duration {
	xdsFirstConnectDelayInit.Do(func() {
		d := time.Second
		if v := os.Getenv("KGW_XDS_FIRST_CONNECT_DELAY"); v != "" {
			parsed, err := time.ParseDuration(v)
			if err == nil {
				d = parsed
			} else {
				logger.Warn("invalid KGW_XDS_FIRST_CONNECT_DELAY; using default", "value", v, "default", time.Second, "error", err)
			}
		}
		xdsFirstConnectDelayNanos.Store(int64(d))
	})
	return time.Duration(xdsFirstConnectDelayNanos.Load())
}

type ConnectedClient struct {
	uniqueClientName string
	// originalRole is the role as presented on the stream's FIRST request,
	// before newStream rewrites the node metadata to the unique cache key.
	// Follow-up SotW requests often omit Node, and go-control-plane then
	// reuses the mutated Node object — so per-request identity re-derivation
	// must start from this pinned role, never from the node's (possibly
	// already-augmented) one, or augmentation compounds and every ACK looks
	// like an identity change.
	originalRole string
	// connectedAt is when the stream was admitted.
	connectedAt time.Time
	// priorXDSVersion marks a stream that reported config held from an earlier
	// stream (a reconnect, or a proxy that outlived a controller restart).
	priorXDSVersion bool
}

func newConnectedClient(uniqueClientName, originalRole string) ConnectedClient {
	return ConnectedClient{
		uniqueClientName: uniqueClientName,
		originalRole:     originalRole,
		connectedAt:      time.Now(),
	}
}

// Certain parts of translation (mainly priority failover) require different translation for
// different clients (for example, two envoys in different AZs).
// This collection represents the unique clients (envoys) that are connected to the xDS server.
// By unique, we mean clients with the same namespace, role, and labels (which include locality).
// This collection is populated using xDS server callbacks. When an envoy connects to us,
// we grab its pod name and namespace from the request's `node.id`.
// We then fetch that pod to get its labels, create a `UniquelyConnectedClient`, and add it to the collection.

type callbacksCollection struct {
	logger           *slog.Logger
	augmentedPods    krt.Collection[LocalityPod]
	clients          map[int64]ConnectedClient
	uniqClientsCount map[string]uint64
	uniqClients      map[string]ir.UniquelyConnectedClient
	// knownLocalClusterBySid records, per STREAM (not per UCC resource name), whether that
	// stream's own EDS subscription has named its expected local-cluster resource (see
	// ir.UniquelyConnectedClient.LocalClusterInfo). Old Envoys never name it (no matching
	// static bootstrap cluster), so entries only ever go false -> true as real requests are
	// observed.
	//
	// This must be tracked per stream rather than per UCC resource name: when pod-locality
	// tracking is disabled (DISABLE_POD_LOCALITY_XDS=true), every replica of a gateway shares
	// one UCC bucket/snapshot (see the augmentedPods-nil branch in add()). During a rolling
	// upgrade some of those replicas' streams may confirm support while sibling streams on the
	// same bucket haven't -- getClients() must only report the bucket as knowing the local
	// cluster once every stream currently in that bucket has confirmed it, or the same
	// full-EDS-withholding this file exists to prevent reappears for the still-old siblings.
	knownLocalClusterBySid map[int64]bool
	// priorXDSVersions counts warm streams per resource name. A count, not a
	// flag, so a shared name (pod locality off) stops being warm when its last
	// warm stream closes, even with cold streams left.
	priorXDSVersions map[string]int
	stateLock        sync.RWMutex

	trigger *krt.RecomputeTrigger
	// hooks are start hooks registered on the owning callbacks; nil without an owner.
	hooks *atomic.Pointer[connectionStartHooks]
}

// connectionStartHooks stores hooks for admission of a key's first stream.
type connectionStartHooks struct {
	admitting func(resourceName string)
	admitted  func(resourceName string)
}

type callbacks struct {
	collection         atomic.Pointer[callbacksCollection]
	hooks              atomic.Pointer[connectionStartHooks]
	streamIDToPeerInfo sync.Map
	xdsAuth            bool
}

type peerInfo struct {
	role   string
	podRef *types.NamespacedName
}

func (x *callbacks) getPeerInfo(sid int64, r *envoy_service_discovery_v3.DiscoveryRequest, usePod bool) (peerInfo, error) {
	var p peerInfo
	if !x.xdsAuth {
		// xDS auth is disabled, retrieve the role from Node metadata
		p.role = roleFromRequest(r)
		if usePod && r.GetNode() != nil {
			p.podRef = new(getRef(r.GetNode()))
		}
		return p, nil
	}

	peerIf, ok := x.streamIDToPeerInfo.Load(sid)
	if !ok {
		return p, fmt.Errorf("xDS peer not found for stream %d", sid)
	}
	caller, ok := peerIf.(*security.Caller)
	if !ok {
		return p, fmt.Errorf("xDS peer %d got unexpected type", sid)
	}
	// Since the pod's ServiceAccount name is the same as the Gateway name, we can use the ServiceAccount to build the role
	p.role = xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, caller.KubernetesInfo.PodNamespace, caller.KubernetesInfo.PodServiceAccount)
	if usePod {
		p.podRef = &types.NamespacedName{
			Name:      caller.KubernetesInfo.PodName,
			Namespace: caller.KubernetesInfo.PodNamespace,
		}
	}
	return p, nil
}

// If augmentedPods is nil, we won't use the pod locality info, and all pods for the same gateway will receive the same config.
type UniquelyConnectedClientsBuilder func(ctx context.Context, krtOpts krtutil.KrtOptions, augmentedPods krt.Collection[LocalityPod]) krt.Collection[ir.UniquelyConnectedClient]

// XDSClientState exposes connection state derived from the xDS callbacks.
//
// HasPriorXDSVersion reports whether a stream of the client opened its
// listener subscription with a held version and an empty nonce: the proxy may
// serve config from an earlier stream or controller. The mark is per resource
// name and lasts until its last held-version stream closes.
//
// RunIfDisconnected runs fn only if no stream is connected under resourceName,
// and reports whether it ran. Admission is excluded until fn returns, and
// go-control-plane creates watches only after admission, so no stream or
// watch of the key starts during fn.
type XDSClientState interface {
	HasPriorXDSVersion(resourceName string) bool
	RunIfDisconnected(resourceName string, fn func()) bool
}

// ConnectionStartHooks is implemented by NewUniquelyConnectedClients' state.
// A key's connection starts when its stream count goes from zero to one.
//
// SetConnectionStartHooks registers two hooks; call it before streams connect.
// A later call replaces them. admitting runs under admission exclusion, with
// no other stream of the key connected and before any watch exists. It must be
// quick and must not take locks held by callers of this state. admitted runs
// after that exclusion but before the first request is answered.
type ConnectionStartHooks interface {
	SetConnectionStartHooks(admitting, admitted func(resourceName string))
}

// SetConnectionStartHooks implements ConnectionStartHooks.
func (x *callbacks) SetConnectionStartHooks(admitting, admitted func(resourceName string)) {
	x.hooks.Store(&connectionStartHooks{admitting: admitting, admitted: admitted})
}

// THIS IS THE SET OF THINGS WE RUN TRANSLATION FOR
// add returned callbacks to the xds server.

// NewUniquelyConnectedClients returns kgateway's client-tracking callbacks, the
// builder for the collection they feed, and the per-client xDS state the
// callbacks record. Extra callbacks are not passed here: chain them ahead of
// these with setup.ChainCallbacks, which delivers every hook to each.
func NewUniquelyConnectedClients(xdsAuth bool) (xdsserver.Callbacks, UniquelyConnectedClientsBuilder, XDSClientState) {
	cb := &callbacks{xdsAuth: xdsAuth}

	envoycb := xdsserver.CallbackFuncs{
		StreamOpenFunc:    cb.OnStreamOpen,
		StreamClosedFunc:  cb.OnStreamClosed,
		StreamRequestFunc: cb.OnStreamRequest,
		FetchRequestFunc:  cb.OnFetchRequest,
	}
	return envoycb, buildCollection(cb), cb
}

func buildCollection(callbacks *callbacks) UniquelyConnectedClientsBuilder {
	return func(ctx context.Context, krtOpts krtutil.KrtOptions, augmentedPods krt.Collection[LocalityPod]) krt.Collection[ir.UniquelyConnectedClient] {
		trigger := krt.NewRecomputeTrigger(true)
		col := &callbacksCollection{
			logger:                 logger,
			augmentedPods:          augmentedPods,
			clients:                make(map[int64]ConnectedClient),
			uniqClientsCount:       make(map[string]uint64),
			uniqClients:            make(map[string]ir.UniquelyConnectedClient),
			knownLocalClusterBySid: make(map[int64]bool),
			priorXDSVersions:       make(map[string]int),
			trigger:                trigger,
			hooks:                  &callbacks.hooks,
		}

		callbacks.collection.Store(col)
		return krt.NewManyFromNothing(
			func(ctx krt.HandlerContext) []ir.UniquelyConnectedClient {
				trigger.MarkDependant(ctx)

				return col.getClients()
			},
			krtOpts.ToOptions("UniqueConnectedClients")...,
		)
	}
}

func (x *callbacks) OnStreamOpen(ctx context.Context, sid int64, _ string) error {
	if x.xdsAuth {
		peer, ok := ctx.Value(xds.PeerCtxKey).(*security.Caller)
		if !ok {
			return fmt.Errorf("got invalid type for xDS peer ctx: %T", ctx.Value(xds.PeerCtxKey))
		}
		x.streamIDToPeerInfo.Store(sid, peer)
	}
	return nil
}

// OnStreamClosed is called immediately prior to closing an xDS stream with a stream ID.
// go-control-plane calls it even when OnStreamOpen failed, so it must tolerate a
// stream it never saw open.
func (x *callbacks) OnStreamClosed(sid int64, _ *envoycorev3.Node) {
	if x.xdsAuth {
		x.streamIDToPeerInfo.Delete(sid)
	}
	c := x.collection.Load()
	if c == nil {
		return
	}
	c.streamClosed(sid)
}

func (x *callbacksCollection) streamClosed(sid int64) {
	x.del(sid)
	// A disconnecting stream can change its bucket's derived KnowsLocalCluster (e.g. it may
	// have been the only sibling still holding a shared bucket back from "all confirmed"), so
	// always recompute -- not just when the whole bucket disappears.
	x.trigger.TriggerRecomputation()
}

func (x *callbacksCollection) del(sid int64) *ir.UniquelyConnectedClient {
	x.stateLock.Lock()
	defer x.stateLock.Unlock()

	c, ok := x.clients[sid]
	delete(x.clients, sid)
	delete(x.knownLocalClusterBySid, sid)
	if ok {
		resourceName := c.uniqueClientName
		if c.priorXDSVersion {
			if n := x.priorXDSVersions[resourceName] - 1; n > 0 {
				x.priorXDSVersions[resourceName] = n
			} else {
				delete(x.priorXDSVersions, resourceName)
			}
		}
		current := x.uniqClientsCount[resourceName]
		x.uniqClientsCount[resourceName] = current - 1
		if current == 1 {
			ucc := x.uniqClients[resourceName]
			delete(x.uniqClientsCount, resourceName)
			delete(x.uniqClients, resourceName)
			delete(x.priorXDSVersions, resourceName)
			return &ucc
		}
	}
	return nil
}

func roleFromRequest(r *envoy_service_discovery_v3.DiscoveryRequest) string {
	return r.GetNode().GetMetadata().GetFields()[xds.RoleKey].GetStringValue()
}

// NormalizeGatewayRole returns a normalized Gateway API proxy identity
// derived from the namespace and gateway name in labels.
// If no gateway name is found, it returns originalRole unchanged.
func NormalizeGatewayRole(originalRole, namespace string, labels map[string]string) string {
	gwName := ir.GatewayNameFromLabels(labels)
	if gwName == "" {
		return originalRole
	}

	return xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, namespace, gwName)
}

// deriveClientIdentity resolves the client's identity (role, namespace,
// labels, locality) from the CURRENT pod state. The pod lookup is a
// point-in-time read outside KRT dependency tracking — nothing re-runs this
// when the pod changes — so callers must re-derive per request (see add) to
// keep a stream's identity from being frozen on data that was stale at
// connect time.
func (x *callbacksCollection) deriveClientIdentity(r *envoy_service_discovery_v3.DiscoveryRequest, peer peerInfo) (*ir.UniquelyConnectedClient, error) {
	var locality ir.PodLocality
	var ns string
	var labels map[string]string
	// see if user wants to use pod locality info; this is only possible when podRef is set in getPeerInfo
	if peer.podRef != nil {
		k := krt.Named{Name: peer.podRef.Name, Namespace: peer.podRef.Namespace}.ResourceName()
		pod := x.augmentedPods.GetKey(k)
		if pod == nil {
			// we need to use the pod locality info, so it's an error if we can't get the pod.
			// Only the node id goes in the message: this error is constructed on
			// every request while the pod is absent (and discarded on established
			// streams), so formatting the full Node proto here would be pure waste.
			return nil, fmt.Errorf("pod not found for node %q", r.GetNode().GetId())
		}
		locality = pod.Locality
		ns = pod.Namespace
		labels = pod.AugmentedLabels
		peer.role = NormalizeGatewayRole(peer.role, ns, labels)
	}
	ucc := ir.NewUniquelyConnectedClient(peer.role, ns, labels, locality)
	return &ucc, nil
}

func (x *callbacksCollection) add(sid int64, r *envoy_service_discovery_v3.DiscoveryRequest, peer peerInfo) (ucName string, newStream, connectionStarted bool, err error) {
	// Identity is re-derived from current pod state on EVERY request, not just
	// the first. A stream's first request can race the pod/node informers
	// (controller start is exactly when every Envoy reconnects), freezing an
	// identity built from stale or incomplete labels/locality — wrong
	// DestinationRule selection and failover priorities for the stream's
	// whole lifetime, with nothing to ever correct it short of an Envoy
	// restart. The derivation is a map lookup plus a label hash, and stream
	// requests are infrequent.
	//
	// deriveClientIdentity does an augmentedPods.GetKey lookup and a label
	// hash (NewUniquelyConnectedClient), neither of which touches our shared
	// maps — so it runs WITHOUT stateLock held. Keeping it inside the
	// critical section would serialize every concurrent xDS stream behind one
	// client's pod lookup. We hold the lock only to read the per-stream entry
	// up front and to mutate the maps at the end. go-control-plane serializes
	// callbacks for a single stream id, so this stream's clients[sid] entry
	// can't change underneath us between those two sections.
	x.stateLock.RLock()
	c, ok := x.clients[sid]
	x.stateLock.RUnlock()
	if ok {
		// Follow-up request on an established stream: when xDS auth is
		// disabled the role comes from node metadata, and the Node object may
		// be the one newStream already mutated to the unique cache key
		// (Envoy omits Node on follow-ups; go-control-plane reuses the first
		// request's). Re-derive from the stream's pinned ORIGINAL role so
		// only genuine pod-state drift (labels, locality, namespace) is
		// detected — never our own augmentation, which would otherwise
		// compound and close the stream on every ACK.
		//
		// Pinning the role does NOT freeze the resource name: the
		// role's real inputs (labels/locality/namespace) are still
		// read from current pod state below and folded into a freshly
		// recomputed resource name via NewUniquelyConnectedClient
		// inside deriveClientIdentity. A changed role yields a
		// changed resource name, which is what the drift check
		// compares.
		peer.role = c.originalRole
	}
	ucc, derr := x.deriveClientIdentity(r, peer)
	if ok {
		// An established stream's identity cannot be changed in place: the
		// snapshot cache key was derived from it when the stream opened (see
		// newStream's node-metadata rewrite). If the freshly derived identity
		// differs, close the stream — Envoy reconnects (with backoff) and
		// re-identifies against current state (OnStreamClosed releases the old
		// identity's refcount). If derivation transiently failed (pod record
		// briefly absent, e.g. an informer blip), keep serving under the
		// established identity rather than churn the stream.
		//
		// LIMITATION: this check runs only when a DiscoveryRequest arrives.
		// SotW Envoys ACK pushed updates and (re)subscribe on warming, so a
		// client receiving config sends requests regularly and a drifted
		// identity heals within a push/ACK cycle. But a stream that receives
		// nothing — e.g. an identity derived from labels so stale that no
		// snapshot is ever published for it — may also never send another
		// request, leaving the wrong identity in place until the stream
		// recycles for another reason (network, controller restart). The
		// go-control-plane callback API offers no way to close a stream
		// outside a request event; bounding worst-case staleness server-side
		// (e.g. gRPC keepalive/MaxConnectionAge) is a deliberate non-goal of
		// this change.
		if derr != nil {
			// Keep serving (see above), but leave a trace: without it, a pod
			// that stays missing (force delete, node lost, discovery-namespace
			// change) is indistinguishable from a healthy request.
			x.logger.Debug("xds client identity re-derivation failed; keeping established identity",
				"client", c.uniqueClientName, "error", derr)
			return c.uniqueClientName, false, false, nil
		}
		if c.uniqueClientName != ucc.ResourceName() {
			x.logger.Info("xds client identity changed; closing stream so the client re-identifies",
				"old", c.uniqueClientName, "new", ucc.ResourceName())
			return "", false, false, fmt.Errorf("xds client identity changed from %q to %q", c.uniqueClientName, ucc.ResourceName())
		}
		return c.uniqueClientName, false, false, nil
	}

	// The version gate runs before derivation errors are surfaced: during
	// informer lag (controller start) the pod lookup fails for every reconnect
	// attempt, and the actionable incompatibility error — and the "envoy proxy
	// connected" log — must not be masked behind generic "pod not found"
	// rejections.
	if err := logAndCheckEnvoyVersion(x.logger, r.GetNode()); err != nil {
		return "", false, false, err
	}
	if derr != nil {
		return "", false, false, derr
	}
	x.logger.Debug("adding xds client", "locality", ucc.Locality, "ns", ucc.Namespace, "labels", ucc.Labels, "role", ucc.Role)
	// TODO: modify request to include the label that are relevant for the client?
	// peer.role is the raw, pre-augmentation role from this first request
	// (deriveClientIdentity normalizes its own copy); pin it for follow-ups.
	c = newConnectedClient(ucc.ResourceName(), peer.role)

	// Re-lock only to publish the new stream into the shared maps. uniqClients
	// and uniqClientsCount are keyed by resource name and shared across all
	// streams, so this section must be serialized; the identity derivation
	// above must not.
	x.stateLock.Lock()
	defer x.stateLock.Unlock()
	x.clients[sid] = c
	currentUnique := x.uniqClientsCount[ucc.ResourceName()]
	x.uniqClientsCount[ucc.ResourceName()] = currentUnique + 1
	if currentUnique == 0 {
		x.uniqClients[ucc.ResourceName()] = *ucc
		// The key's connection starts. The admission lock keeps other streams of
		// the key out while the hook runs.
		if h := x.connectionStartHooks(); h != nil && h.admitting != nil {
			h.admitting(ucc.ResourceName())
		}
	}
	return c.uniqueClientName, true, currentUnique == 0, nil
}

// OnStreamRequest is called once a request is received on a stream.
// Returning an error will end processing and close the stream. OnStreamClosed will still be called.
func (x *callbacks) OnStreamRequest(sid int64, r *envoy_service_discovery_v3.DiscoveryRequest) error {
	// A request carrying ErrorDetail is a NACK: the client rejected the
	// previous response for this type URL and keeps serving its
	// last-accepted config. This is the only point where a rejection is
	// visible to the control plane, so record it before any gating below.
	recordNackIfAny(roleFromRequest(r), r)

	c := x.collection.Load()
	if c == nil {
		return errors.New("kgateway not initialized")
	}

	peerInfo, err := x.getPeerInfo(sid, r, c.augmentedPods != nil)
	if err != nil {
		return err
	}
	// check that this collection only handles kgateway clients
	if !xds.IsKubeGatewayCacheKey(peerInfo.role) {
		return nil
	}

	return c.newStream(sid, r, peerInfo)
}

func (x *callbacksCollection) connectionStartHooks() *connectionStartHooks {
	if x.hooks == nil {
		return nil
	}
	return x.hooks.Load()
}

func (x *callbacksCollection) newStream(sid int64, r *envoy_service_discovery_v3.DiscoveryRequest, peer peerInfo) error {
	ucc, isNewStream, connectionStarted, err := x.add(sid, r, peer)
	if err != nil {
		x.logger.Debug("error processing xds client", "error", err)
		return err
	}
	if ucc == "" {
		return fmt.Errorf("got empty unique client name for sid %d", sid)
	}
	x.observeLocalClusterRequest(sid, ucc, r)
	if hasPriorXDSVersion(r) {
		x.markPriorXDSVersion(sid, ucc)
	}

	nodeMd := r.GetNode().GetMetadata()
	if nodeMd == nil {
		nodeMd = &structpb.Struct{}
	}
	if nodeMd.GetFields() == nil {
		nodeMd.Fields = map[string]*structpb.Value{}
	}

	x.logger.Debug("augmenting role in node metadata", "resource_name", ucc)
	// NOTE: this changes the role to include the unique client. This is coupled
	// with how the snapshot is inserted to the cache for the proxy - it needs to be done with
	// the unique client resource name as well.
	nodeMd.GetFields()[xds.RoleKey] = structpb.NewStringValue(ucc)
	r.GetNode().Metadata = nodeMd
	if isNewStream {
		// A new stream joining an existing bucket (not just a brand new bucket) can change
		// that bucket's derived KnowsLocalCluster (it starts as an unconfirmed member), so
		// recompute on every new stream, not only isNewUCC.
		x.trigger.TriggerRecomputation()
	}
	if h := x.connectionStartHooks(); connectionStarted && h != nil && h.admitted != nil {
		h.admitted(ucc)
	}
	if delay := xdsFirstConnectDelay(); isNewStream && delay > 0 {
		// See xdsFirstConnectDelay: give per-client translation a head start
		// before go-control-plane creates this stream's first watch.
		time.Sleep(delay)
	}
	return nil
}

// hasPriorXDSVersion reports whether r is an initial listener request carrying
// a previously accepted version. Only listeners count: warmth protects served
// config, and a proxy whose stream broke during CDS warming serves none, so it
// starts within the first-publish budget.
func hasPriorXDSVersion(r *envoy_service_discovery_v3.DiscoveryRequest) bool {
	return r.GetTypeUrl() == resource.ListenerType && r.GetResponseNonce() == "" && r.GetVersionInfo() != ""
}

// markPriorXDSVersion marks stream sid warm. Each stream counts once toward
// its resource name.
func (x *callbacksCollection) markPriorXDSVersion(sid int64, resourceName string) {
	x.stateLock.Lock()
	defer x.stateLock.Unlock()
	c, ok := x.clients[sid]
	if !ok || c.priorXDSVersion {
		return
	}
	c.priorXDSVersion = true
	x.clients[sid] = c
	x.priorXDSVersions[resourceName]++
}

func (x *callbacks) HasPriorXDSVersion(resourceName string) bool {
	c := x.collection.Load()
	if c == nil {
		return false
	}
	return c.hasPriorXDSVersion(resourceName)
}

func (x *callbacksCollection) hasPriorXDSVersion(resourceName string) bool {
	x.stateLock.RLock()
	defer x.stateLock.RUnlock()
	return x.priorXDSVersions[resourceName] > 0
}

// HasColdStreamSince reports whether a cold stream has been connected under
// resourceName for at least d. A cold proxy sharing a warm name has no
// listeners until the shared snapshot publishes, so the gate uses this to lift
// the warm withhold. It scans all streams: call it only at publication
// deadlines.
func (x *callbacks) HasColdStreamSince(resourceName string, d time.Duration) bool {
	c := x.collection.Load()
	if c == nil {
		return false
	}
	return c.hasColdStreamSince(resourceName, d)
}

func (x *callbacksCollection) hasColdStreamSince(resourceName string, d time.Duration) bool {
	x.stateLock.RLock()
	defer x.stateLock.RUnlock()
	cutoff := time.Now().Add(-d)
	for _, c := range x.clients {
		if c.uniqueClientName == resourceName && !c.priorXDSVersion && !c.connectedAt.After(cutoff) {
			return true
		}
	}
	return false
}

// RunIfDisconnected implements XDSClientState. Before collection construction,
// no stream has been admitted, so fn runs.
func (x *callbacks) RunIfDisconnected(resourceName string, fn func()) bool {
	c := x.collection.Load()
	if c == nil {
		fn()
		return true
	}
	return c.runIfDisconnected(resourceName, fn)
}

// runIfDisconnected uses add's admission write lock.
func (x *callbacksCollection) runIfDisconnected(resourceName string, fn func()) bool {
	x.stateLock.Lock()
	defer x.stateLock.Unlock()
	if x.uniqClientsCount[resourceName] > 0 {
		return false
	}
	fn()
	return true
}

func (x *callbacksCollection) getClients() []ir.UniquelyConnectedClient {
	x.stateLock.RLock()
	defer x.stateLock.RUnlock()

	// A bucket knows about the local cluster only if EVERY stream currently mapped to it has
	// individually confirmed support -- a single un-confirmed sibling stream (sharing the
	// bucket because pod-locality tracking is disabled) holds the whole bucket back. Track
	// only the buckets held back, so the (usually much larger) uniqClients map only needs one
	// pass below instead of two.
	unconfirmedBuckets := make(map[string]struct{})
	for sid, c := range x.clients {
		if !x.knownLocalClusterBySid[sid] {
			unconfirmedBuckets[c.uniqueClientName] = struct{}{}
		}
	}

	clients := make([]ir.UniquelyConnectedClient, 0, len(x.uniqClients))
	for name, c := range x.uniqClients {
		_, unconfirmed := unconfirmedBuckets[name]
		c.KnowsLocalCluster = !unconfirmed
		clients = append(clients, c)
	}
	return clients
}

// observeLocalClusterRequest records whether this STREAM's own EDS subscription has named its
// expected local-cluster resource (see ir.UniquelyConnectedClient.LocalClusterInfo). It only
// ever transitions a stream from unknown to known, and only triggers recomputation on that
// transition — repeat ACKs on an already-known stream are a no-op.
func (x *callbacksCollection) observeLocalClusterRequest(sid int64, uccName string, r *envoy_service_discovery_v3.DiscoveryRequest) {
	if r.GetTypeUrl() != resource.EndpointType {
		return
	}
	// tryConfirmLocalCluster's lock must be released before triggering recomputation: the
	// registered collection handler (getClients) takes stateLock.RLock, and since
	// sync.RWMutex isn't reentrant, calling TriggerRecomputation while still holding the
	// write lock here would deadlock if it's ever invoked synchronously on this goroutine.
	if x.tryConfirmLocalCluster(sid, uccName, r) {
		x.trigger.TriggerRecomputation()
	}
}

// tryConfirmLocalCluster marks sid as knowing about the local cluster if this request proves
// it, returning whether a false -> true transition actually happened.
func (x *callbacksCollection) tryConfirmLocalCluster(sid int64, uccName string, r *envoy_service_discovery_v3.DiscoveryRequest) bool {
	x.stateLock.Lock()
	defer x.stateLock.Unlock()

	if x.knownLocalClusterBySid[sid] {
		return false
	}
	ucc, ok := x.uniqClients[uccName]
	if !ok {
		return false
	}
	localClusterName, _, _ := ucc.LocalClusterInfo()
	if localClusterName == "" {
		return false
	}
	if !slices.Contains(r.GetResourceNames(), localClusterName) {
		return false
	}
	x.knownLocalClusterBySid[sid] = true
	return true
}

// OnFetchRequest is called for each Fetch request. Returning an error will end processing of the
// request and respond with an error.
func (x *callbacks) OnFetchRequest(ctx context.Context, r *envoy_service_discovery_v3.DiscoveryRequest) error {
	if x.xdsAuth {
		return errors.New("OnFetchRequest not supported when xDS auth is enabled")
	}

	role := r.GetNode().GetMetadata().GetFields()[xds.RoleKey].GetStringValue()
	// check that this collection only handles kgateway clients
	// TODO remove this check if it's no longer needed
	if !xds.IsKubeGatewayCacheKey(role) {
		return nil
	}
	c := x.collection.Load()
	if c == nil {
		return errors.New("kgateway not initialized")
	}
	return c.fetchRequest(ctx, r)
}

func (x *callbacksCollection) fetchRequest(_ context.Context, r *envoy_service_discovery_v3.DiscoveryRequest) error {
	// nothing special to do in a fetch request, as we don't need to maintain state
	if x.augmentedPods == nil {
		return nil
	}

	podRef := getRef(r.GetNode())
	ucc, err := x.deriveClientIdentity(r, peerInfo{role: roleFromRequest(r), podRef: &podRef})
	if err != nil {
		return err
	}

	nodeMd := r.GetNode().GetMetadata()
	if nodeMd == nil {
		nodeMd = &structpb.Struct{}
	}
	if nodeMd.GetFields() == nil {
		nodeMd.Fields = map[string]*structpb.Value{}
	}

	x.logger.Debug("augmenting role in node metadata", "resource_name", ucc.ResourceName())
	// NOTE: this changes the role to include the unique client. This is coupled
	// with how the snapshot is inserted to the cache for the proxy - it needs to be done with
	// the unique client resource name as well.
	nodeMd.GetFields()[xds.RoleKey] = structpb.NewStringValue(ucc.ResourceName())
	r.GetNode().Metadata = nodeMd
	return nil
}

// minEnvoy{Minor,Patch}Version is the minimum Envoy version (under major 1) required to connect.
// Old Envoy is not forward-compatible with newer control plane xDS schemas; new Envoy is
// backward-compatible with older control planes, so we enforce a floor here to prevent a broken
// state during helm upgrades.
const (
	minEnvoyMinorVersion = 37
	minEnvoyPatchVersion = 2
)

func logAndCheckEnvoyVersion(logger *slog.Logger, node *envoycorev3.Node) error {
	if node == nil {
		return nil
	}

	versionStr := "unknown"
	var major, minor, patch uint32
	knownVersion := false

	switch v := node.GetUserAgentVersionType().(type) {
	case *envoycorev3.Node_UserAgentBuildVersion:
		sv := v.UserAgentBuildVersion.GetVersion()
		if sv != nil {
			major = sv.GetMajorNumber()
			minor = sv.GetMinorNumber()
			patch = sv.GetPatch()
			versionStr = fmt.Sprintf("%d.%d.%d", major, minor, patch)
			knownVersion = true
		}
	case *envoycorev3.Node_UserAgentVersion:
		versionStr = v.UserAgentVersion
	}

	logger.Info("envoy proxy connected", "version", versionStr, "node_id", node.GetId(), "user_agent", node.GetUserAgentName())

	if !knownVersion {
		return nil
	}

	if major < 1 || (major == 1 && minor < minEnvoyMinorVersion) || (major == 1 && minor == minEnvoyMinorVersion && patch < minEnvoyPatchVersion) {
		return fmt.Errorf("envoy version %s is not compatible with this control plane: minimum required version is 1.%d.%d; upgrade envoy before upgrading the control plane",
			versionStr, minEnvoyMinorVersion, minEnvoyPatchVersion)
	}
	return nil
}

func getRef(node *envoycorev3.Node) types.NamespacedName {
	nns := node.GetId()
	split := strings.SplitN(nns, ".", 2)
	if len(split) != 2 {
		return types.NamespacedName{}
	}
	return types.NamespacedName{
		Name:      split[0],
		Namespace: split[1],
	}
}
