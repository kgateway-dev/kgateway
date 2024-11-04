package krtcollections

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"
	"sync/atomic"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/solo-io/gloo/projects/gateway2/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	xdsserver "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/structpb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"
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
type UniqlyConnectedClient struct {
	Role      string
	Labels    map[string]string
	Locality  PodLocality
	Namespace string

	// modified role that includes the namespace and the hash of the labels.
	// we set the client's role to this value in the node metadata. so the snapshot key in the cache
	// should also be set to this value.
	resourceName string
}

func (c UniqlyConnectedClient) ResourceName() string {
	return c.resourceName
}

var _ krt.Equaler[UniqlyConnectedClient] = new(UniqlyConnectedClient)

func (c UniqlyConnectedClient) Equals(k UniqlyConnectedClient) bool {
	return c.Role == k.Role && c.Namespace == k.Namespace && c.Locality == k.Locality && maps.Equal(c.Labels, k.Labels)
}

func labeledRole(role string, labels map[string]string) string {
	return fmt.Sprintf("%s%s%d", role, xds.KeyDelimiter, utils.HashLabels(labels))
}

func newUniqlyConnectedClient(node *envoy_config_core_v3.Node, ns string, labels map[string]string, locality PodLocality) UniqlyConnectedClient {
	role := node.GetMetadata().GetFields()[xds.RoleKey].GetStringValue()
	snapshotKey := labeledRole(role, labels)
	return UniqlyConnectedClient{
		Role:         role,
		Namespace:    ns,
		Locality:     locality,
		Labels:       labels,
		resourceName: fmt.Sprintf("%s%s%s", snapshotKey, xds.KeyDelimiter, ns),
	}
}

type callbacksCollection struct {
	logger           *zap.Logger
	augmentedPods    krt.Collection[LocalityPod]
	clients          map[int64]ConnectedClient
	uniqClientsCount map[string]uint64
	uniqClients      map[string]UniqlyConnectedClient
	stateLock        sync.RWMutex

	trigger *krt.RecomputeTrigger
}

type callbacks struct {
	collection atomic.Pointer[callbacksCollection]
}

type UniquelyConnectedClientsBulider func(ctx context.Context, augmentedPods krt.Collection[LocalityPod]) krt.Collection[UniqlyConnectedClient]

// THIS IS THE SET OF THINGS WE RUN TRANSLATION FOR
// add returned callbacks to the xds server.

func NewUniquelyConnectedClients() (xdsserver.Callbacks, UniquelyConnectedClientsBulider) {
	cb := &callbacks{}
	return cb, buildCollection(cb)
}

func buildCollection(callbacks *callbacks) UniquelyConnectedClientsBulider {
	return func(ctx context.Context, augmentedPods krt.Collection[LocalityPod]) krt.Collection[UniqlyConnectedClient] {
		trigger := krt.NewRecomputeTrigger(true)
		col := &callbacksCollection{
			logger:           contextutils.LoggerFrom(ctx).Desugar(),
			augmentedPods:    augmentedPods,
			clients:          make(map[int64]ConnectedClient),
			uniqClientsCount: make(map[string]uint64),
			uniqClients:      make(map[string]UniqlyConnectedClient),
			trigger:          trigger,
		}

		callbacks.collection.Store(col)
		return krt.NewManyFromNothing(
			func(ctx krt.HandlerContext) []UniqlyConnectedClient {
				trigger.MarkDependant(ctx)

				return col.getClients()
			},
			krt.WithName("UniqueConnectedClients"),
		)
	}
}

var _ xdsserver.Callbacks = new(callbacks)

// OnStreamOpen is called once an xDS stream is open with a stream ID and the type URL (or "" for ADS).
// Returning an error will end processing and close the stream. OnStreamClosed will still be called.
func (x *callbacks) OnStreamOpen(_ context.Context, _ int64, _ string) error {
	return nil
}

// OnStreamClosed is called immediately prior to closing an xDS stream with a stream ID.
func (x *callbacks) OnStreamClosed(sid int64) {
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

func (x *callbacksCollection) del(sid int64) *UniqlyConnectedClient {
	x.stateLock.Lock()
	defer x.stateLock.Unlock()

	c, ok := x.clients[sid]
	delete(x.clients, sid)
	if ok {
		resouceName := c.uniqueClientName
		current := x.uniqClientsCount[resouceName]
		x.uniqClientsCount[resouceName] = current - 1
		if current == 1 {
			ucc := x.uniqClients[resouceName]
			delete(x.uniqClientsCount, resouceName)
			delete(x.uniqClients, resouceName)
			return &ucc
		}
	}
	return nil
}

func (x *callbacksCollection) add(sid int64, r *envoy_service_discovery_v3.DiscoveryRequest) (string, bool, error) {

	var pod *LocalityPod
	if r.GetNode() != nil {
		podRef := getRef(r.GetNode())
		k := krt.Key[LocalityPod](krt.Named{Name: podRef.Name, Namespace: podRef.Namespace}.ResourceName())
		pod = x.augmentedPods.GetKey(k)
	}
	addedNew := false
	x.stateLock.Lock()
	defer x.stateLock.Unlock()
	c, ok := x.clients[sid]
	if !ok {
		if pod == nil {
			// error if we can't get the pod
			return "", false, fmt.Errorf("pod not found for node %v", r.GetNode())
		}
		x.logger.Debug("adding xds client", zap.Any("locality", pod.Locality), zap.String("ns", pod.Namespace), zap.Any("labels", pod.AugmentedLabels))
		// TODO: modify request to include the label that are relevant for the client?
		ucc := newUniqlyConnectedClient(r.GetNode(), pod.Namespace, pod.AugmentedLabels, pod.Locality)
		c = newConnectedClient(ucc.resourceName)
		x.clients[sid] = c
		currentUnique := x.uniqClientsCount[ucc.resourceName]
		x.uniqClientsCount[ucc.resourceName] = currentUnique + 1
		if currentUnique == 0 {
			x.uniqClients[ucc.resourceName] = ucc
			addedNew = true
		}
	}
	return c.uniqueClientName, addedNew, nil

}

// OnStreamRequest is called once a request is received on a stream.
// Returning an error will end processing and close the stream. OnStreamClosed will still be called.
func (x *callbacks) OnStreamRequest(sid int64, r *envoy_service_discovery_v3.DiscoveryRequest) error {
	role := r.GetNode().GetMetadata().GetFields()[xds.RoleKey].GetStringValue()
	// as gloo-edge and ggv2 share a control plane, check that this collection only handles ggv2 clients
	if !xds.IsKubeGatewayCacheKey(role) {
		return nil
	}
	c := x.collection.Load()
	if c == nil {
		return errors.New("ggv2 not initialized")
	}
	return c.newStream(sid, r)
}

func (x *callbacksCollection) newStream(sid int64, r *envoy_service_discovery_v3.DiscoveryRequest) error {
	ucc, isNew, err := x.add(sid, r)
	if err != nil {
		x.logger.Debug("error processing xds client", zap.Error(err))
		return err
	}
	if ucc != "" {
		nodeMd := r.GetNode().GetMetadata()
		if nodeMd == nil {
			nodeMd = &structpb.Struct{}
		}
		if nodeMd.GetFields() == nil {
			nodeMd.Fields = map[string]*structpb.Value{}
		}

		x.logger.Debug("augmenting role in node metadata", zap.String("resourceName", ucc))
		// NOTE: this changes the role to include the unique client. This is coupled
		// with how the snapshot is inserted to the cache for the proxy - it needs to be done with
		// the unique client resource name as well.
		nodeMd.GetFields()[xds.RoleKey] = structpb.NewStringValue(ucc)
		r.GetNode().Metadata = nodeMd
		if isNew {
			x.trigger.TriggerRecomputation()
		}
	}
	return nil
}

func (x *callbacksCollection) getClients() []UniqlyConnectedClient {
	x.stateLock.RLock()
	defer x.stateLock.RUnlock()
	clients := make([]UniqlyConnectedClient, 0, len(x.uniqClients))
	for _, c := range x.uniqClients {
		clients = append(clients, c)
	}
	return clients
}

// OnStreamResponse is called immediately prior to sending a response on a stream.
func (x *callbacks) OnStreamResponse(_ int64, _ *envoy_service_discovery_v3.DiscoveryRequest, _ *envoy_service_discovery_v3.DiscoveryResponse) {
}

// OnFetchRequest is called for each Fetch request. Returning an error will end processing of the
// request and respond with an error.
func (x *callbacks) OnFetchRequest(ctx context.Context, r *envoy_service_discovery_v3.DiscoveryRequest) error {

	role := r.GetNode().GetMetadata().GetFields()[xds.RoleKey].GetStringValue()
	// as gloo-edge and ggv2 share a control plane, check that this collection only handles ggv2 clients
	if !xds.IsKubeGatewayCacheKey(role) {
		return nil
	}
	c := x.collection.Load()
	if c == nil {
		return errors.New("ggv2 not initialized")
	}
	return c.fetchRequest(ctx, r)
}

func (x *callbacksCollection) fetchRequest(_ context.Context, r *envoy_service_discovery_v3.DiscoveryRequest) error {
	var pod *LocalityPod
	podRef := getRef(r.GetNode())
	k := krt.Key[LocalityPod](krt.Named{Name: podRef.Name, Namespace: podRef.Namespace}.ResourceName())
	pod = x.augmentedPods.GetKey(k)
	ucc := newUniqlyConnectedClient(r.GetNode(), pod.Namespace, pod.AugmentedLabels, pod.Locality)

	nodeMd := r.GetNode().GetMetadata()
	if nodeMd == nil {
		nodeMd = &structpb.Struct{}
	}
	if nodeMd.GetFields() == nil {
		nodeMd.Fields = map[string]*structpb.Value{}
	}

	x.logger.Debug("augmenting role in node metadata", zap.String("resourceName", ucc.resourceName))
	// NOTE: this changes the role to include the unique client. This is coupled
	// with how the snapshot is inserted to the cache for the proxy - it needs to be done with
	// the unique client resource name as well.
	nodeMd.GetFields()[xds.RoleKey] = structpb.NewStringValue(ucc.resourceName)
	r.GetNode().Metadata = nodeMd
	return nil
}

// OnFetchResponse is called immediately prior to sending a response.
func (x *callbacks) OnFetchResponse(_ *envoy_service_discovery_v3.DiscoveryRequest, _ *envoy_service_discovery_v3.DiscoveryResponse) {
}

func (x *callbacksCollection) Synced() krt.Syncer {
	return &simpleSyncer{}
}

// GetKey returns an object by its key, if present. Otherwise, nil is returned.

func (x *callbacksCollection) GetKey(k krt.Key[UniqlyConnectedClient]) *UniqlyConnectedClient {
	x.stateLock.RLock()
	defer x.stateLock.RUnlock()
	u, ok := x.uniqClients[string(k)]
	if ok {
		return &u
	}
	return nil
}

// List returns all objects in the collection.
// Order of the list is undefined.

func (x *callbacksCollection) List() []UniqlyConnectedClient { return x.getClients() }

type simpleSyncer struct{}

func (s *simpleSyncer) WaitUntilSynced(stop <-chan struct{}) bool {
	return true
}

func (s *simpleSyncer) HasSynced() bool {
	return true
}

func getRef(node *envoy_config_core_v3.Node) types.NamespacedName {
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
