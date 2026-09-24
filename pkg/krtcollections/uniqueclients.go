package krtcollections

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
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
				slog.Warn("invalid KGW_XDS_FIRST_CONNECT_DELAY; using default", "value", v, "default", time.Second, "error", err)
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
}

func newConnectedClient(uniqueClientName, originalRole string) ConnectedClient {
	return ConnectedClient{
		uniqueClientName: uniqueClientName,
		originalRole:     originalRole,
	}
}

// Certain parts of translation (mainly priority failover) require different translation for
// different clients (for example, 2 envoys on different AZs).
// This collection represents the unique clients (envoys) that are connected to the xds server.
// by unique we mean same namespace, role, labels (which include locality).
// This collection is populated using xds server callbacks. When an envoy connects to us,
// we grab it's pod name/namesspace from the requests node->id.
// We then fetch that pod to get its labels, create a UniqlyConnectedClient and it them to the collection.

type callbacksCollection struct {
	logger           *slog.Logger
	augmentedPods    krt.Collection[LocalityPod]
	clients          map[int64]ConnectedClient
	uniqClientsCount map[string]uint64
	uniqClients      map[string]ir.UniqlyConnectedClient
	stateLock        sync.RWMutex

	trigger *krt.RecomputeTrigger
}

type callbacks struct {
	collection         atomic.Pointer[callbacksCollection]
	extraXDSCallbacks  xdsserver.Callbacks
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
type UniquelyConnectedClientsBulider func(ctx context.Context, krtOpts krtutil.KrtOptions, augmentedPods krt.Collection[LocalityPod]) krt.Collection[ir.UniqlyConnectedClient]

// THIS IS THE SET OF THINGS WE RUN TRANSLATION FOR
// add returned callbacks to the xds server.

func NewUniquelyConnectedClients(
	extraXDSCallbacks xdsserver.Callbacks,
	xdsAuth bool,
) (xdsserver.Callbacks, UniquelyConnectedClientsBulider) {
	cb := &callbacks{
		extraXDSCallbacks: extraXDSCallbacks,
		xdsAuth:           xdsAuth,
	}

	envoycb := xdsserver.CallbackFuncs{
		StreamOpenFunc:    cb.OnStreamOpen,
		StreamClosedFunc:  cb.OnStreamClosed,
		StreamRequestFunc: cb.OnStreamRequest,
		FetchRequestFunc:  cb.OnFetchRequest,
	}
	return envoycb, buildCollection(cb)
}

func buildCollection(callbacks *callbacks) UniquelyConnectedClientsBulider {
	return func(ctx context.Context, krtOpts krtutil.KrtOptions, augmentedPods krt.Collection[LocalityPod]) krt.Collection[ir.UniqlyConnectedClient] {
		trigger := krt.NewRecomputeTrigger(true)
		col := &callbacksCollection{
			logger:           logger,
			augmentedPods:    augmentedPods,
			clients:          make(map[int64]ConnectedClient),
			uniqClientsCount: make(map[string]uint64),
			uniqClients:      make(map[string]ir.UniqlyConnectedClient),
			trigger:          trigger,
		}

		callbacks.collection.Store(col)
		return krt.NewManyFromNothing(
			func(ctx krt.HandlerContext) []ir.UniqlyConnectedClient {
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
func (x *callbacks) OnStreamClosed(sid int64, node *envoycorev3.Node) {
	if x.extraXDSCallbacks != nil {
		x.extraXDSCallbacks.OnStreamClosed(sid, node)
	}

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
	ucc := x.del(sid)
	if ucc != nil {
		x.trigger.TriggerRecomputation()
	}
}

func (x *callbacksCollection) del(sid int64) *ir.UniqlyConnectedClient {
	x.stateLock.Lock()
	defer x.stateLock.Unlock()

	c, ok := x.clients[sid]
	delete(x.clients, sid)
	if ok {
		resourceName := c.uniqueClientName
		current := x.uniqClientsCount[resourceName]
		x.uniqClientsCount[resourceName] = current - 1
		if current == 1 {
			ucc := x.uniqClients[resourceName]
			delete(x.uniqClientsCount, resourceName)
			delete(x.uniqClients, resourceName)
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
	if labels == nil {
		return originalRole
	}

	gwName := labels[wellknown.GatewayNameAnnotation]
	if gwName == "" {
		gwName = labels[wellknown.GatewayNameLabel]
	}
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
func (x *callbacksCollection) deriveClientIdentity(r *envoy_service_discovery_v3.DiscoveryRequest, peer peerInfo) (*ir.UniqlyConnectedClient, error) {
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
	ucc := ir.NewUniqlyConnectedClient(peer.role, ns, labels, locality)
	return &ucc, nil
}

func (x *callbacksCollection) add(sid int64, r *envoy_service_discovery_v3.DiscoveryRequest, peer peerInfo) (ucName string, newStream, newUCC bool, err error) {
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
	// hash (NewUniqlyConnectedClient), neither of which touches our shared
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
		// recomputed resource name via NewUniqlyConnectedClient
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
	addedNew := false
	currentUnique := x.uniqClientsCount[ucc.ResourceName()]
	x.uniqClientsCount[ucc.ResourceName()] = currentUnique + 1
	if currentUnique == 0 {
		x.uniqClients[ucc.ResourceName()] = *ucc
		addedNew = true
	}
	return c.uniqueClientName, !ok, addedNew, nil
}

// OnStreamRequest is called once a request is received on a stream.
// Returning an error will end processing and close the stream. OnStreamClosed will still be called.
func (x *callbacks) OnStreamRequest(sid int64, r *envoy_service_discovery_v3.DiscoveryRequest) error {
	if x.extraXDSCallbacks != nil {
		if err := x.extraXDSCallbacks.OnStreamRequest(sid, r); err != nil {
			return err
		}
	}

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

func (x *callbacksCollection) newStream(sid int64, r *envoy_service_discovery_v3.DiscoveryRequest, peer peerInfo) error {
	ucc, isNewStream, isNewUCC, err := x.add(sid, r, peer)
	if err != nil {
		x.logger.Debug("error processing xds client", "error", err)
		return err
	}
	if ucc == "" {
		return fmt.Errorf("got empty unique client name for sid %d", sid)
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
	if isNewUCC {
		x.trigger.TriggerRecomputation()
	}
	if delay := xdsFirstConnectDelay(); isNewStream && delay > 0 {
		// See xdsFirstConnectDelay: give per-client translation a head start
		// before go-control-plane creates this stream's first watch.
		time.Sleep(delay)
	}
	return nil
}

func (x *callbacksCollection) getClients() []ir.UniqlyConnectedClient {
	x.stateLock.RLock()
	defer x.stateLock.RUnlock()
	clients := make([]ir.UniqlyConnectedClient, 0, len(x.uniqClients))
	for _, c := range x.uniqClients {
		clients = append(clients, c)
	}
	return clients
}

// OnFetchRequest is called for each Fetch request. Returning an error will end processing of the
// request and respond with an error.
func (x *callbacks) OnFetchRequest(ctx context.Context, r *envoy_service_discovery_v3.DiscoveryRequest) error {
	if x.xdsAuth {
		return fmt.Errorf("OnFetchRequest not supported when xDS auth is enabled")
	}
	if x.extraXDSCallbacks != nil {
		if err := x.extraXDSCallbacks.OnFetchRequest(ctx, r); err != nil {
			return err
		}
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
