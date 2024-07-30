package iosnapshot

import (
	"context"
	"sync"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	corev1 "k8s.io/api/core/v1"

	"github.com/hashicorp/go-multierror"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	crdv1 "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// History represents an object that maintains state about the running system
// The ControlPlane will use the Setters to update the last known state,
// and the Getters will be used by the Admin Server
type History interface {
	// SetApiSnapshot sets the latest Edge ApiSnapshot.
	SetApiSnapshot(latestInput *v1snap.ApiSnapshot)

	// SetKubeGatewayClient sets the client to use for Kubernetes CRUD operations when
	// Kubernetes Gateway integration is enabled. If this is not set, then it is assumed
	// that Kubernetes Gateway integration is not enabled, and no Kubernetes Gateway
	// resources will be returned from `GetInputSnapshot`.
	SetKubeGatewayClient(kubeGatewayClient client.Client)

	// GetInputSnapshot returns all resources in the Edge input snapshot, and if Kubernetes
	// Gateway integration is enabled, it additionally returns all resources on the cluster
	// with types specified by `kubeGatewayGvks`.
	GetInputSnapshot(ctx context.Context) SnapshotResponseData

	// GetEdgeApiSnapshot returns all resources in the Edge input snapshot
	GetEdgeApiSnapshot(ctx context.Context) SnapshotResponseData

	// GetProxySnapshot returns the Proxies generated for all components.
	GetProxySnapshot(ctx context.Context) SnapshotResponseData

	// GetXdsSnapshot returns the entire cache of xDS snapshots
	// NOTE: This contains sensitive data, as it is the exact inputs that used by Envoy
	GetXdsSnapshot(ctx context.Context) SnapshotResponseData
}

// HistoryFactoryParameters are the inputs used to create a History object
type HistoryFactoryParameters struct {
	Settings *gloov1.Settings
	Cache    cache.SnapshotCache
}

// HistoryFactory is a function that produces a History object
type HistoryFactory func(params HistoryFactoryParameters) History

// GetHistoryFactory returns a default HistoryFactory implementation
func GetHistoryFactory() HistoryFactory {
	return func(params HistoryFactoryParameters) History {
		return NewHistory(params.Cache, params.Settings, KubeGatewayDefaultGVKs)
	}
}

// NewHistory returns an implementation of the History interface
//   - `cache` is the control plane's xDS snapshot cache
//   - `settings` specifies the Settings for this control plane instance
//   - `kubeGatewayGvks` specifies the list of resource types to return in the input snapshot when
//     Kubernetes Gateway integration is enabled. For example, this may include Gateway API
//     resources, Portal resources, or other resources specific to the Kubernetes Gateway integration.
//     If not set, then only Edge ApiSnapshot resources will be returned from `GetInputSnapshot`.
func NewHistory(cache cache.SnapshotCache, settings *gloov1.Settings, kubeGatewayGvks []schema.GroupVersionKind) History {
	return &historyImpl{
		latestApiSnapshot: nil,
		xdsCache:          cache,
		settings:          settings,
		kubeGatewayClient: nil,
		kubeGatewayGvks:   kubeGatewayGvks,
	}
}

type historyImpl struct {
	// TODO:
	// 	We rely on a mutex to prevent races reading/writing the data for this object
	//	We should instead use channels to coordinate this
	sync.RWMutex
	latestApiSnapshot *v1snap.ApiSnapshot
	xdsCache          cache.SnapshotCache
	settings          *gloov1.Settings
	kubeGatewayClient client.Client
	// kubeGatewayGvks is the list of GVKs that the historyImpl will return when GetInputSnapshot is invoked
	kubeGatewayGvks []schema.GroupVersionKind
}

// SetApiSnapshot sets the latest input ApiSnapshot
func (h *historyImpl) SetApiSnapshot(latestApiSnapshot *v1snap.ApiSnapshot) {
	// Setters are called by the running Control Plane, so we perform the update in a goroutine to prevent
	// any contention/issues, from impacting the runtime of the system
	go func() {
		h.Lock()
		defer h.Unlock()

		h.latestApiSnapshot = latestApiSnapshot
	}()
}

func (h *historyImpl) SetKubeGatewayClient(kubeGatewayClient client.Client) {
	// Setters are called by the running Control Plane, so we perform the update in a goroutine to prevent
	// any contention/issues, from impacting the runtime of the system
	go func() {
		h.Lock()
		defer h.Unlock()

		h.kubeGatewayClient = kubeGatewayClient
	}()
}

func (h *historyImpl) GetEdgeApiSnapshot(_ context.Context) SnapshotResponseData {
	snap := h.getRedactedApiSnapshot()

	m, err := apiSnapshotToGenericMap(snap)
	if err != nil {
		return errorSnapshotResponse(err)
	}

	return completeSnapshotResponse(m)
}

// GetInputSnapshot gets the input snapshot for all components.
func (h *historyImpl) GetInputSnapshot(ctx context.Context) SnapshotResponseData {
	snap := h.getRedactedApiSnapshot()

	// Proxies are defined on the ApiSnapshot, but are not considered part of the
	// "input snapshot" since they are the product (output) of translation
	snap.Proxies = nil

	// If kubernetes gateway integration is enabled, we remove resource types from the ApiSnapshot
	// that are shared between the edge and kube gateway controllers. Since the kube gateway controller
	// watches all resources of the relevant types on the cluster (rather than only ones in the snapshot),
	// the resources returned by getKubeGatewayResources below is a superset of what's in the edge api snapshot.
	// So we remove the duplicate resource types from the api snapshot here.
	kubeGatewayClient := h.getKubeGatewayClientSafe()
	if kubeGatewayClient != nil {
		// kube gateway integration is enabled
		snap.RouteOptions = nil
		snap.VirtualHostOptions = nil
		snap.AuthConfigs = nil
		snap.Ratelimitconfigs = nil
		snap.Secrets = nil
		snap.Upstreams = nil
	}

	// get the resources from the edge api snapshot
	resources, err := snapshotToKubeResources(snap)
	if err != nil {
		return errorSnapshotResponse(err)
	}

	// get settings, which is not part of the api snapshot
	if h.settings != nil {
		settings, err := settingsToKubeResource(h.settings)
		if err != nil {
			return errorSnapshotResponse(err)
		}
		resources = append(resources, *settings)
	}

	// get the resources that the kubernetes gateway controller watches
	// (if kube gateway integration is enabled)
	kubeResources, err := h.getKubeGatewayResources(ctx)
	if err != nil {
		return errorSnapshotResponse(err)
	}
	resources = append(resources, kubeResources...)

	sortResources(resources)
	return completeSnapshotResponse(resources)
}

func (h *historyImpl) GetProxySnapshot(_ context.Context) SnapshotResponseData {
	snap := h.getRedactedApiSnapshot()

	onlyProxies := &v1snap.ApiSnapshot{
		Proxies: snap.Proxies,
	}

	resources, err := snapshotToKubeResources(onlyProxies)
	if err != nil {
		return errorSnapshotResponse(err)
	}

	sortResources(resources)
	return completeSnapshotResponse(resources)
}

// GetXdsSnapshot returns the entire cache of xDS snapshots
// NOTE: This contains sensitive data, as it is the exact inputs that used by Envoy
func (h *historyImpl) GetXdsSnapshot(_ context.Context) SnapshotResponseData {
	cacheKeys := h.xdsCache.GetStatusKeys()
	cacheEntries := make(map[string]interface{}, len(cacheKeys))

	for _, k := range cacheKeys {
		xdsSnapshot, err := h.xdsCache.GetSnapshot(k)
		if err != nil {
			cacheEntries[k] = err.Error()
		} else {
			cacheEntries[k] = xdsSnapshot
		}
	}

	return completeSnapshotResponse(cacheEntries)
}

// getRedactedApiSnapshot gets an in-memory copy of the ApiSnapshot
// Any sensitive data contained in the Snapshot will either be explicitly redacted
// or entirely excluded
// NOTE: Redaction is somewhat of an expensive operation, so we have a few options for how to approach it:
//
//  1. Perform it when a new ApiSnapshot is received from the Control Plane
//
//  2. Perform it on demand, when an ApiSnapshot is requested
//
//  3. Perform it on demand, when an ApiSnapshot is requested, but store a local cache for future requests.
//     That cache would be invalidated each time a new ApiSnapshot is received.
//
//     Given that the rate of requests for the ApiSnapshot <<< the frequency of updates of an ApiSnapshot by the Control Plane,
//     in this first pass we opt to take approach #2.
func (h *historyImpl) getRedactedApiSnapshot() *v1snap.ApiSnapshot {
	snap := h.getApiSnapshotSafe()

	redactApiSnapshot(snap)
	return snap
}

// getApiSnapshotSafe gets a clone of the latest ApiSnapshot
func (h *historyImpl) getApiSnapshotSafe() *v1snap.ApiSnapshot {
	h.RLock()
	defer h.RUnlock()
	if h.latestApiSnapshot == nil {
		return &v1snap.ApiSnapshot{}
	}

	// This clone is critical!!
	// We do this to ensure the following cases:
	//	1. Modifications to this snapshot, by the admin server, DO NOT impact the Control Plane
	//	2. Modifications to this snapshot by a single request, DO NOT interfere with other requests
	clone := h.latestApiSnapshot.Clone()
	return &clone
}

// getKubeGatewayResources returns the list of resources specific to the Kubernetes Gateway integration.
// Since the Kubernetes Gateway controller does not have the concept of input snapshots or watch
// namespaces, we return all resources on the cluster of the given types.
func (h *historyImpl) getKubeGatewayResources(ctx context.Context) ([]crdv1.Resource, error) {
	kubeGatewayClient := h.getKubeGatewayClientSafe()
	if kubeGatewayClient == nil {
		// No client has been set, so the Kubernetes Gateway integration has not been enabled
		return nil, nil
	}

	var resources []crdv1.Resource
	var errs *multierror.Error
	for _, gvk := range h.kubeGatewayGvks {
		gvkResources, err := h.listResourcesForGvk(ctx, gvk)
		if err != nil {
			// We intentionally aggregate the errors so that we can return a "best effort" set of
			// resources, and one error doesn't lead to the entire set of GVKs being short-circuited
			errs = multierror.Append(errs, err)
		} else {
			resources = append(resources, gvkResources...)
		}
	}

	return resources, errs.ErrorOrNil()
}

func (h *historyImpl) listResourcesForGvk(ctx context.Context, gvk schema.GroupVersionKind) ([]crdv1.Resource, error) {
	var resources []crdv1.Resource

	// populate an unstructured list for each resource type
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk)
	err := h.kubeGatewayClient.List(ctx, list)
	if err != nil {
		return nil, err
	}

	var errs *multierror.Error

	// convert each Unstructured to a Resource so that the final list can be merged with the
	// Edge API resource list
	for _, uns := range list.Items {
		out := crdv1.Resource{}
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(uns.Object, &out)
		if err != nil {
			errs = multierror.Append(errs, err)
		} else {
			sanitizeResource(&out)
			resources = append(resources, out)
		}
	}
	return resources, errs.ErrorOrNil()
}

// getKubeGatewayClientSafe gets the Kubernetes client used for CRUD operations
// on resources used in the Kubernetes Gateway integration
func (h *historyImpl) getKubeGatewayClientSafe() client.Client {
	h.RLock()
	defer h.RUnlock()
	return h.kubeGatewayClient
}

// redactApiSnapshot accepts an ApiSnapshot, and mutates it to remove sensitive data.
// It is critical that data which is exposed by this component is redacted,
// so that customers can feel comfortable sharing the results with us.
//
// NOTE: This is an extremely naive implementation. It is intended as a first pass to get this API
// into the hands of the field.As we iterate on this component, we can use some of the redaction
// utilities in `/pkg/utils/syncutil`.
func redactApiSnapshot(snap *v1snap.ApiSnapshot) {
	snap.Secrets.Each(func(element *gloov1.Secret) {
		redactSecretData(element)
	})

	// See `pkg/utils/syncutil/log_redactor.StringifySnapshot` for an explanation for
	// why we redact Artifacts
	snap.Artifacts.Each(func(element *gloov1.Artifact) {
		redactArtifactData(element)
	})
}

// redactSecretData modifies the secret to remove any sensitive information
// The structure of a Secret in Gloo Gateway does not lend itself to easily redact data in different places.
// As a result, we perform a primitive redaction method, where we maintain the metadata, and remove the entire spec
func redactSecretData(element *gloov1.Secret) {
	element.Kind = nil

	redactGlooResourceMetadata(element.GetMetadata())
}

const (
	redactedString = "<redacted>"
)

// redactArtifactData modifies the artifact to remove any sensitive information
func redactArtifactData(element *gloov1.Artifact) {
	for k := range element.GetData() {
		element.GetData()[k] = redactedString
	}

	redactGlooResourceMetadata(element.GetMetadata())
}

// redactGlooResourceMetadata modifies the metadata to remove any sensitive information
// ref: https://github.com/solo-io/skv2/blob/1583cb716c04eb3f8d01ecb179b0deeabaa6e42b/contrib/pkg/snapshot/redact.go#L20-L26
func redactGlooResourceMetadata(meta *core.Metadata) {
	for key, _ := range meta.GetAnnotations() {
		if key == corev1.LastAppliedConfigAnnotation {
			meta.GetAnnotations()[key] = redactedString
			break
		}
	}
}

// sanitizeResource modifies the Object to remove any unwanted fields
func sanitizeResource(resource *crdv1.Resource) {
	// ManagedFields is noise on the object, that is not relevant to the Admin API, so we sanitize it
	resource.ManagedFields = nil
}
