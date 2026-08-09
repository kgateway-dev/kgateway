// Derived from https://github.com/agentgateway/agentgateway controller/pkg/syncer/status_syncer.go (Apache 2.0).

package statussync

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/ptr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

const (
	// Retry configuration constants for status updates. Conflicts and NotFound are not
	// retried here: the status collection re-enqueues the resource once the informer
	// delivers the newer object (live status != desired status), so lost writes self-heal.
	maxRetryAttempts = 5
	retryDelay       = 100 * time.Millisecond
)

// ResourceStatusSyncer writes the desired status for a single resource kind.
type ResourceStatusSyncer interface {
	ApplyStatus(ctx context.Context, obj Resource)
}

// Writer is a generic ResourceStatusSyncer that writes status via the istio kclient,
// i.e. the same client/informer cache that translation reads from.
type Writer[O controllers.ComparableObject, S any] struct {
	// Name for logging
	Name string

	// Client reads the current object (from the shared informer cache) and writes status.
	Client kclient.Client[O]

	// Desired builds the desired status from the latest object and report state. Returning
	// false suppresses the write. It is invoked for every retry so neither the object nor
	// report snapshot is retained in the work queue.
	Desired func(current O) (S, bool)

	// Build constructs the object to pass to UpdateStatus. Only the status and minimal
	// ObjectMeta (name, namespace, resourceVersion) should be set: the API server ignores
	// spec on status writes, and passing the resourceVersion ensures stale data is rejected.
	Build func(om metav1.ObjectMeta, s S) O

	// GetStatus extracts the live status from the current object. When set, a write is
	// skipped if the (merged) desired status already matches the live status.
	GetStatus func(o O) S

	// Merge, when set, merges the desired status with the current object at write time.
	// Used to preserve multi-writer fields owned by other controllers or subsystems
	// (e.g. route status parents, policy ancestors, gateway addresses).
	Merge func(current O, desired S) S

	// OnSync, when set, is called once per ApplyStatus invocation for which Desired returns
	// true. current is the last object read from the informer and status the last merged
	// status. Used to record status sync metrics.
	OnSync func(res Resource, current O, status S, took time.Duration, err error)

	// NotReady, when set, re-queues resources this writer's client cannot see yet. Required
	// for correctness whenever Client is a delayed client, because its Get returns nil until
	// its own informer loads and nothing upstream re-fires afterwards. See NotReadyRequeuer.
	NotReady *NotReadyRequeuer
}

var _ ResourceStatusSyncer = Writer[*gwv1.Gateway, *gwv1.GatewayStatus]{}

// RetryStatusWrite runs attempt with the standard status-write retry policy: 5 attempts
// with exponential backoff, aborted on ctx cancellation. attempt must re-read the current
// object each time and swallow (return nil for) conflicts and NotFound, which self-heal
// via re-enqueue; only transient errors (throttling, 5xx, network) should be returned.
// Custom ResourceStatusSyncer implementations should wrap their write in this so transient
// failures are not silently dropped: after a failed write nothing changes on the informer,
// so no event is guaranteed to re-enqueue the resource.
func RetryStatusWrite(ctx context.Context, attempt func() error) error {
	return retry.Do(attempt,
		retry.Context(ctx),
		retry.Attempts(maxRetryAttempts),
		retry.Delay(retryDelay),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true),
	)
}

func (w Writer[O, S]) ApplyStatus(ctx context.Context, obj Resource) {
	log := logger.With("kind", w.Name, "resource", obj.NamespacedName.String())
	start := time.Now()
	var lastCurrent O
	var lastMerged S
	hasDesired := false
	invisible := false
	err := RetryStatusWrite(ctx, func() error {
		hasDesired = false
		// Fetch the current object so we can preserve status written by other controllers or
		// subsystems, and suppress writes that would be no-ops.
		current := w.Client.Get(obj.Name, obj.Namespace)
		if controllers.IsNil(current) {
			// Either the resource was deleted (a harmless race) or this client's informer
			// has not loaded yet. The two are indistinguishable here, so hand it to the
			// requeuer, which bounds how long we keep waiting.
			invisible = true
			log.Debug("resource not found, skipping status update")
			return nil
		}
		invisible = false
		lastCurrent = current

		desired, ok := w.Desired(current)
		if !ok {
			log.Debug("resource has no desired status, skipping status update")
			return nil
		}
		hasDesired = true
		merged := desired
		if w.Merge != nil {
			merged = w.Merge(current, desired)
		}
		lastMerged = merged

		if w.GetStatus != nil && krt.Equal(w.GetStatus(current), merged) {
			log.Debug("status already up to date, skipping status update")
			return nil
		}

		// Write with the informer's current resourceVersion so stale data is rejected;
		// conflicts are expected and self-heal via re-enqueue.
		_, err := w.Client.UpdateStatus(w.Build(metav1.ObjectMeta{
			Name:            obj.Name,
			Namespace:       obj.Namespace,
			ResourceVersion: current.GetResourceVersion(),
		}, merged))
		if err != nil {
			if apierrors.IsConflict(err) {
				// This is normal. The raw collection will re-enqueue the write once the
				// informer delivers the newer object.
				log.Debug("updating stale status, skipping", "error", err)
				return nil
			}
			if apierrors.IsNotFound(err) {
				// ignore status write after resource was deleted.
				log.Debug("resource not found, skipping status update", "error", err)
				return nil
			}
			log.Error("error updating status", "error", err)
			return err
		}
		log.Debug("updated status")
		return nil
	})
	if err != nil {
		log.Error("failed to sync status after retries", "error", err)
	}
	if invisible {
		w.NotReady.Schedule(obj)
	} else {
		w.NotReady.Done(obj)
	}
	if hasDesired && w.OnSync != nil {
		w.OnSync(obj, lastCurrent, lastMerged, time.Since(start), err)
	}
}

// MergePolicyAncestorStatuses preserves PolicyStatus ancestors owned by other controllers,
// replacing only the entries owned by ourControllerName with the desired entries.
// Publishing an empty desired list therefore clears our stale entries without touching others.
//
// The merged list is capped at the Gateway API limit (16 ancestors), which the API server
// enforces via CRD schema. Entries owned by other controllers are never dropped in favor
// of ours: our entries are truncated first. reports.BuildPolicyStatus caps the desired list
// at the same limit before it gets here and follows the same ownership policy, so the two
// caps agree on which entries survive.
func MergePolicyAncestorStatuses(ourControllerName string, existing, desired []gwv1.PolicyAncestorStatus) []gwv1.PolicyAncestorStatus {
	return mergeOwnedStatusEntries(
		ourControllerName, existing, desired,
		func(a gwv1.PolicyAncestorStatus) string { return string(a.ControllerName) },
		func(a gwv1.PolicyAncestorStatus) gwv1.ParentReference { return a.AncestorRef },
		reports.MaxPolicyStatusAncestors, "PolicyStatus.ancestors",
	)
}

// MergeRouteParentStatuses preserves RouteStatus parents owned by other controllers,
// replacing only the entries owned by ourControllerName with the desired entries.
// Publishing an empty desired list therefore clears our stale entries without touching others.
//
// The merged list is capped at the Gateway API limit (32 parents), which the API server
// enforces via CRD schema. Entries owned by other controllers are never dropped in favor
// of ours: our entries are truncated first.
func MergeRouteParentStatuses(ourControllerName string, existing, desired []gwv1.RouteParentStatus) []gwv1.RouteParentStatus {
	return mergeOwnedStatusEntries(
		ourControllerName, existing, desired,
		func(p gwv1.RouteParentStatus) string { return string(p.ControllerName) },
		func(p gwv1.RouteParentStatus) gwv1.ParentReference { return p.ParentRef },
		reports.MaxRouteStatusParents, "RouteStatus.parents",
	)
}

// mergeOwnedStatusEntries implements the shared merge for the two Gateway API status lists
// that several controllers write to. PolicyStatus.ancestors and RouteStatus.parents differ
// only in element type and the name of their ref field.
//
// The published order is canonical: entries are sorted by ParentString, the same key istio
// and kgateway's own report builders use. That matters because the writer suppresses no-op
// writes with a plain equality check. If we published in an arbitrary order, a peer
// controller that rewrites the whole list in sorted order — which is exactly what these
// builders do — would disagree with us on ordering alone, and the two of us would rewrite
// the list back and forth forever.
func mergeOwnedStatusEntries[T any](
	ourControllerName string,
	existing, desired []T,
	controllerOf func(T) string,
	refOf func(T) gwv1.ParentReference,
	limit int,
	field string,
) []T {
	out := make([]T, 0, len(existing)+len(desired))

	// Preserve any entries not owned by our controller.
	for _, e := range existing {
		if controllerOf(e) != ourControllerName {
			out = append(out, e)
		}
	}

	// Only add entries owned by our controller from the desired status.
	ours := make([]T, 0, len(desired))
	for _, d := range desired {
		if controllerOf(d) == ourControllerName {
			ours = append(ours, d)
		}
	}

	// Order ours deterministically before the cap, so which of them survives truncation does
	// not depend on map/set iteration upstream.
	slices.SortFunc(ours, func(a, b T) int {
		if c := cmp.Compare(controllerOf(a), controllerOf(b)); c != 0 {
			return c
		}
		return compareParentReference(refOf(a), refOf(b))
	})

	// Foreign entries first so the cap truncates ours before anyone else's.
	out = append(out, ours...)
	out = capMergedStatusEntries(out, limit, field)

	// Decorate before sorting: ParentString formats a string, and a comparator would
	// re-format both operands on every one of the O(n log n) comparisons, on every write
	// attempt (including retries) of every resource.
	keyed := make([]keyedStatusEntry[T], len(out))
	for i, e := range out {
		keyed[i] = keyedStatusEntry[T]{key: reports.ParentString(refOf(e)), entry: e}
	}
	slices.SortStableFunc(keyed, func(a, b keyedStatusEntry[T]) int {
		return strings.Compare(a.key, b.key)
	})
	for i, k := range keyed {
		out[i] = k.entry
	}
	return out
}

// keyedStatusEntry pairs a status list entry with its precomputed sort key.
type keyedStatusEntry[T any] struct {
	key   string
	entry T
}

// capMergedStatusEntries truncates a merged status list to the Gateway API schema limit,
// so writes are not rejected by the API server when entries owned by other controllers
// already fill (or nearly fill) the list. Merged lists place other controllers' entries
// first, so truncating the tail drops our entries before anyone else's. This must be
// applied by the merge itself (not at the write site only) so desired-status
// normalization stays byte-identical to the written result.
func capMergedStatusEntries[T any](entries []T, limit int, field string) []T {
	if len(entries) <= limit {
		return entries
	}
	// We cannot represent the surplus entries within the API limit, so log the
	// truncation explicitly.
	logger.Warn("truncating merged status entries to Gateway API limit",
		"field", field,
		"total_entries", len(entries),
		"dropped_entries", len(entries)-limit,
	)
	return entries[:limit]
}

func compareParentReference(a, b gwv1.ParentReference) int {
	// ParentReference includes pointer fields with defaults (Group is the Gateway API group,
	// Kind is Gateway). Canonicalize those defaults so nil vs explicitly-set default values
	// don't introduce ordering churn.
	if c := cmp.Compare(ptr.OrDefault(a.Group, gwv1.Group(gwv1.GroupName)), ptr.OrDefault(b.Group, gwv1.Group(gwv1.GroupName))); c != 0 {
		return c
	}
	if c := cmp.Compare(ptr.OrDefault(a.Kind, gwv1.Kind(wellknown.GatewayKind)), ptr.OrDefault(b.Kind, gwv1.Kind(wellknown.GatewayKind))); c != 0 {
		return c
	}
	if c := cmp.Compare(ptr.OrEmpty(a.Namespace), ptr.OrEmpty(b.Namespace)); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Name, b.Name); c != 0 {
		return c
	}
	if c := cmp.Compare(ptr.OrEmpty(a.SectionName), ptr.OrEmpty(b.SectionName)); c != 0 {
		return c
	}
	return comparePortNumberPtr(a.Port, b.Port)
}

// comparePortNumberPtr sorts an unset port before port 0, so it cannot be replaced with
// ptr.OrEmpty: the two are distinct values in a ParentReference.
func comparePortNumberPtr(a, b *gwv1.PortNumber) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	default:
		return cmp.Compare(int(*a), int(*b))
	}
}
