// ON_EXPERIMENTAL_PROMOTION : Delete this file, along with xlistenerset_status_test.go,
// inject_listener_ports_test.go, and the XListenerSetGVK writer registration in
// initStatusInfra. Nothing else reaches into the legacy status path: the promoted ListenerSet
// writer is a standard statussync.Writer that shares only the report builder and the metrics
// hook with this one, and both of those stay.
// Ref: https://github.com/kgateway-dev/kgateway/issues/12827

package proxy_syncer

import (
	"context"
	"encoding/json"

	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

// xListenerSetStatusSyncer is the status writer registered for the legacy experimental
// XListenerSet GVK. Reads are identical to the promoted ListenerSet writer -- same
// normalized collection, same report reducer, same builder -- and only the write differs:
// the legacy CRD is served under its own GVR and its schema still requires a per-listener
// port that gwv1.ListenerSetStatus no longer carries, so it goes out as a dynamic merge
// patch rather than a typed UpdateStatus.
//
// It exists only to hold the context: the writer it delegates to is a plain
// statussync.Writer, but Writer.UpdateStatus is handed an ObjectMeta and a status and
// carries no context of its own, and the dynamic client needs one. Building the writer per
// call is a handful of closures on a path that only runs for legacy objects.
type xListenerSetStatusSyncer struct {
	col     krt.Collection[*gwv1.ListenerSet]
	client  apiclient.Client
	reports krt.Collection[statussync.ResourceReports]
}

var _ statussync.ResourceStatusSyncer = &xListenerSetStatusSyncer{}

func (s *xListenerSetStatusSyncer) ApplyStatus(ctx context.Context, res statussync.Resource) {
	s.writer(ctx).ApplyStatus(ctx, res)
}

// writer is the legacy writer as the standard status pipeline sees it. Exposed as a method
// so tests can assert the shared invariants on it -- CheckWriterIdempotent above all -- with
// the same writer ApplyStatus runs.
func (s *xListenerSetStatusSyncer) writer(ctx context.Context) statussync.Writer[*gwv1.ListenerSet, gwv1.ListenerSetStatus] {
	return statussync.Writer[*gwv1.ListenerSet, gwv1.ListenerSetStatus]{
		Name:         "xListenerSet",
		Current:      statussync.CollectionSource(s.col),
		Desired:      listenerSetDesired(wellknown.XListenerSetGVK, s.reports),
		UpdateStatus: s.patchStatus(ctx),
		GetStatus:    func(o *gwv1.ListenerSet) gwv1.ListenerSetStatus { return o.Status },
		OnSync:       listenerSetStatusMetricsHook(),
	}
}

// patchStatus merge-patches the status subresource of a legacy XListenerSet through the
// dynamic client, injecting the per-listener port required by the legacy CRD schema.
//
// The patch body carries metadata.resourceVersion so the API server rejects the write when
// the object has moved on, matching the optimistic concurrency the promoted path gets from
// UpdateStatus. Without it a merge patch applies unconditionally, so a status built from a
// stale read silently overwrites a newer one and nothing re-enqueues to correct it.
//
// The spec listeners the ports come from are re-read here rather than threaded through the
// status, because there is no field of gwv1.ListenerSetStatus to thread them through. It is
// the same collection Writer.Current reads, so the only way the read comes back empty is
// that the object was deleted mid-write; the patch would then be rejected anyway, and
// stamping every listener with the fallback port is a worse guess than not writing.
func (s *xListenerSetStatusSyncer) patchStatus(ctx context.Context) func(metav1.ObjectMeta, gwv1.ListenerSetStatus) error {
	return func(om metav1.ObjectMeta, desired gwv1.ListenerSetStatus) error {
		current := s.col.GetKey(om.Namespace + "/" + om.Name)
		if current == nil || *current == nil {
			logger.Debug("legacy listener set no longer present, skipping status update",
				"resource", om.Namespace+"/"+om.Name)
			return nil
		}
		statusMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&desired)
		if err != nil {
			return err
		}
		injectListenerPorts(statusMap, (*current).Spec.Listeners)
		data, err := json.Marshal(map[string]any{
			"metadata": map[string]any{"resourceVersion": om.ResourceVersion},
			"status":   statusMap,
		})
		if err != nil {
			return err
		}
		// Conflicts and NotFound are not handled here: Writer.ApplyStatus recognizes both and
		// skips the write, since the raw collection re-enqueues once the informer delivers the
		// newer object.
		_, err = s.client.Dynamic().Resource(wellknown.XListenerSetGVR).Namespace(om.Namespace).
			Patch(ctx, om.Name, types.MergePatchType, data, metav1.PatchOptions{}, "status")
		return err
	}
}

// legacyPortFallback fills the legacy schema's required status port when a listener's
// protocol requires an explicit port but none is set, or when a status entry matches no
// spec listener, matching the v2.2.4 fallback behaviour. 65535 is a valid port — the
// highest — so it satisfies the schema's 1-65535 range; it is only a recognizable
// placeholder, not a sentinel the schema would reject elsewhere. It appears solely in
// written status and never programs a listener, so a genuine listener on port 65535 is
// unaffected (its status entry is then indistinguishable from the fallback, which is the
// cost of the legacy schema requiring a port we do not always have).
const legacyPortFallback int64 = 65535

// injectListenerPorts adds the "port" field to each entry in statusMap["listeners"]
// by looking up the matching listener in specListeners by name.
// This is needed because gwv1.ListenerEntryStatus no longer carries Port, but the
// legacy XListenerSet CRD schema still requires it.
// Listeners whose name does not match any spec entry receive legacyPortFallback
// so that the patch payload always satisfies the schema's required constraint.
func injectListenerPorts(statusMap map[string]any, specListeners []gwv1.ListenerEntry) {
	listeners, ok := statusMap["listeners"].([]any)
	if !ok {
		return
	}

	// Precompute name→port to avoid O(n²) scan.
	portByName := make(map[string]int64, len(specListeners))
	for _, spec := range specListeners {
		port, err := kubeutils.DetectListenerPortNumber(spec.Protocol, spec.Port)
		if err != nil {
			port = gwv1.PortNumber(legacyPortFallback)
		}
		portByName[string(spec.Name)] = int64(port)
	}

	for i, entry := range listeners {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		name, _ := entryMap["name"].(string)
		port, matched := portByName[name]
		if !matched {
			// No corresponding spec entry; use the fallback so the patch
			// payload still satisfies the schema's required port constraint.
			port = legacyPortFallback
		}
		entryMap["port"] = port
		listeners[i] = entryMap
	}
}
