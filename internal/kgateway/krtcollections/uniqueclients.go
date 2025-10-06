package krtcollections

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"google.golang.org/protobuf/types/known/structpb"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/security"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

type ConnectedClient struct {
	uniqueClientName string
}

func newConnectedClient(uniqueClientName string) ConnectedClient {
	return ConnectedClient{
		uniqueClientName: uniqueClientName,
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
			p.podRef = ptr.To(getRef(r.GetNode()))
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

func (x *callbacksCollection) add(sid int64, r *envoy_service_discovery_v3.DiscoveryRequest, peer peerInfo) (string, bool, error) {
	var pod *LocalityPod
	// see if user wants to use pod locality info; this is only possible when podRef is set in getPeerInfo
	if peer.podRef != nil {
		k := krt.Named{Name: peer.podRef.Name, Namespace: peer.podRef.Namespace}.ResourceName()
		pod = x.augmentedPods.GetKey(k)
	}
	addedNew := false
	x.stateLock.Lock()
	defer x.stateLock.Unlock()
	c, ok := x.clients[sid]
	if !ok {
		var locality ir.PodLocality
		var ns string
		var labels map[string]string
		if peer.podRef != nil {
			if pod == nil {
				// we need to use the pod locality info, so it's an error if we can't get the pod
				return "", false, fmt.Errorf("pod not found for node %v", r.GetNode())
			} else {
				locality = pod.Locality
				ns = pod.Namespace
				labels = pod.AugmentedLabels
			}
		}
		x.logger.Debug("adding xds client", "locality", locality, "ns", ns, "labels", labels, "role", peer.role)
		// TODO: modify request to include the label that are relevant for the client?
		ucc := ir.NewUniqlyConnectedClient(peer.role, ns, labels, locality)
		c = newConnectedClient(ucc.ResourceName())
		x.clients[sid] = c
		currentUnique := x.uniqClientsCount[ucc.ResourceName()]
		x.uniqClientsCount[ucc.ResourceName()] = currentUnique + 1
		if currentUnique == 0 {
			x.uniqClients[ucc.ResourceName()] = ucc
			addedNew = true
		}
	}
	return c.uniqueClientName, addedNew, nil
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
	ucc, isNew, err := x.add(sid, r, peer)
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
	if isNew {
		x.trigger.TriggerRecomputation()
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

	var pod *LocalityPod
	podRef := getRef(r.GetNode())
	k := krt.Named{Name: podRef.Name, Namespace: podRef.Namespace}.ResourceName()
	pod = x.augmentedPods.GetKey(k)
	ucc := ir.NewUniqlyConnectedClient(roleFromRequest(r), pod.Namespace, pod.AugmentedLabels, pod.Locality)

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
