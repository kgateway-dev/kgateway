package iosnapshot

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/schemes"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	crdv1 "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/hashicorp/go-multierror"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// History represents an object that maintains state about the running system
// The ControlPlane will use the Setters to update the last known state,
// and the Getters will be used by the Admin Server
type History interface {
	// SetApiSnapshot sets the latest Edge ApiSnapshot.
	SetApiSnapshot(latestInput *v1snap.ApiSnapshot)

	// GetInputSnapshot returns all resources in the Edge input snapshot, and if Kubernetes
	// Gateway integration is enabled, it additionally returns all resources on the cluster
	// with types specified by `inputSnapshotGvks`.
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
	Settings                    *gloov1.Settings
	Cache                       cache.SnapshotCache
	EnableK8sGatewayIntegration bool
}

// HistoryFactory is a function that produces a History object
type HistoryFactory func(params HistoryFactoryParameters) History

// GetHistoryFactory returns a default HistoryFactory implementation
func GetHistoryFactory() HistoryFactory {
	return func(params HistoryFactoryParameters) History {
		var kubeClient client.Client

		cfg, err := kubeutils.GetRestConfigWithKubeContext("")
		if err == nil {
			cli, err := client.New(cfg, client.Options{
				Scheme: schemes.DefaultScheme(),
			})
			if err == nil {
				kubeClient = cli
			}
		}

		// By default, only return the GVKs for using Gloo Gateway, with purely the Edge Gateway APIs
		var gvks = EdgeOnlyInputSnapshotGVKs
		if params.EnableK8sGatewayIntegration {
			gvks = CompleteInputSnapshotGVKs
		}

		return NewHistory(params.Cache, params.Settings, kubeClient, gvks)
	}
}

// NewHistory returns an implementation of the History interface
//   - `cache` is the control plane's xDS snapshot cache
//   - `settings` specifies the Settings for this control plane instance
//   - `inputSnapshotGvks` specifies the list of resource types to return in the input snapshot when
//     Kubernetes Gateway integration is enabled. For example, this may include Gateway API
//     resources, Portal resources, or other resources specific to the Kubernetes Gateway integration.
//     If not set, then only Edge ApiSnapshot resources will be returned from `GetInputSnapshot`.
func NewHistory(cache cache.SnapshotCache, settings *gloov1.Settings, kubeClient client.Client, kubeGatewayGvks []schema.GroupVersionKind) History {
	return &historyImpl{
		latestApiSnapshot:   nil,
		xdsCache:            cache,
		settings:            settings,
		inputSnapshotClient: kubeClient,
		inputSnapshotGvks:   kubeGatewayGvks,
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

	// The InputSnapshot API is really a pass through to the Kubernetes API Server
	// Below are properties that are used to configure the behavior of GetInputSnapshot

	inputSnapshotClient client.Client
	inputSnapshotGvks   []schema.GroupVersionKind
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

func (h *historyImpl) GetEdgeApiSnapshot(_ context.Context) SnapshotResponseData {
	snap := h.getRedactedApiSnapshot()
	return completeSnapshotResponse(snap)
}

// GetInputSnapshot gets the input snapshot for all components.
func (h *historyImpl) GetInputSnapshot(ctx context.Context) SnapshotResponseData {
	if h.inputSnapshotClient == nil {
		return errorSnapshotResponse(eris.New("No kubernetes Client found for InputSnapshot"))
	}

	var objects []client.Object
	var errs *multierror.Error
	for _, gvk := range h.inputSnapshotGvks {
		gvkResources, err := h.listObjectsForGvk(ctx, h.inputSnapshotClient, gvk)
		if err != nil {
			// We intentionally aggregate the errors so that we can return a "best effort" set of
			// resources, and one error doesn't lead to the entire set of GVKs being short-circuited
			errs = multierror.Append(errs, err)
		}
		objects = append(objects, gvkResources...)
	}
	sortResources(objects)

	return SnapshotResponseData{
		Data:  objects,
		Error: errs.ErrorOrNil(),
	}
}

func (h *historyImpl) GetProxySnapshot(_ context.Context) SnapshotResponseData {
	snap := h.getRedactedApiSnapshot()

	var resources []crdv1.Resource
	var errs *multierror.Error

	for _, proxy := range snap.Proxies {
		kubeProxy, err := gloov1.ProxyCrd.KubeResource(proxy)
		if err != nil {
			// We intentionally aggregate the errors so that we can return a "best effort" set of
			// resources, and one error doesn't lead to the entire set of GVKs being short-circuited
			errs = multierror.Append(errs, err)
		}
		resources = append(resources, *kubeProxy)
	}

	return SnapshotResponseData{
		Data:  resources,
		Error: errs.ErrorOrNil(),
	}
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

func (h *historyImpl) listObjectsForGvk(ctx context.Context, cli client.Client, gvk schema.GroupVersionKind) ([]client.Object, error) {
	var objects []client.Object

	// populate an unstructured list for each resource type
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk)
	err := cli.List(ctx, list)
	if err != nil {
		return nil, err
	}

	var errs *multierror.Error

	// convert each Unstructured to a client.Object
	for _, uns := range list.Items {
		realObj, err := cli.Scheme().New(gvk)
		if err != nil {
			errs = multierror.Append(errs, err)
			continue
		}
		clientObj, ok := realObj.(client.Object)
		if !ok {
			errs = multierror.Append(errs, eris.New(fmt.Sprintf("%s could not be converted into client.Object", gvk)))
			continue
		}

		err = runtime.DefaultUnstructuredConverter.FromUnstructured(uns.Object, clientObj)
		if err != nil {
			errs = multierror.Append(errs, err)
			continue
		}

		redactClientObject(clientObj)
		objects = append(objects, clientObj)
	}

	return objects, errs.ErrorOrNil()
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
		redactGlooSecretData(element)
	})

	// See `pkg/utils/syncutil/log_redactor.StringifySnapshot` for an explanation for
	// why we redact Artifacts
	snap.Artifacts.Each(func(element *gloov1.Artifact) {
		redactGlooArtifactData(element)
	})
}

// sortResources sorts resources by gvk, namespace, and name
func sortResources(resources []client.Object) {
	slices.SortStableFunc(resources, func(a, b client.Object) int {
		return cmp.Or(
			cmp.Compare(a.GetObjectKind().GroupVersionKind().Version, b.GetObjectKind().GroupVersionKind().Version),
			cmp.Compare(a.GetObjectKind().GroupVersionKind().Kind, b.GetObjectKind().GroupVersionKind().Kind),
			cmp.Compare(a.GetNamespace(), b.GetNamespace()),
			cmp.Compare(a.GetName(), b.GetName()),
		)
	})
}

// apiSnapshotToGenericMap converts an ApiSnapshot into a generic map
// Since maps do not guarantee ordering, we do not attempt to sort these resources, as we do four []crdv1.Resource
func apiSnapshotToGenericMap(snap *v1snap.ApiSnapshot) (map[string]interface{}, error) {
	genericMap := map[string]interface{}{}

	if snap == nil {
		return genericMap, nil
	}

	jsn, err := json.Marshal(snap)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(jsn, &genericMap); err != nil {
		return nil, err
	}
	return genericMap, nil
}
