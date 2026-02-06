package inferencepool

import (
	"fmt"
	"strings"
	"time"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	// defaultInfPoolStatusKind is the Kind defined by the default InferencePool
	// parent status condition.
	defaultInfPoolStatusKind = "Status"
	// defaultInfPoolStatusName is the Name defined by the default InferencePool
	// parent status condition.
	defaultInfPoolStatusName = "default"
)

// buildRegisterCallback returns a function that registers all handlers for the
// Inference Extension plugin.
func buildRegisterCallback(
	commonCol *collections.CommonCollections,
	cli kclient.Client[*inf.InferencePool],
	bcol krt.Collection[ir.BackendObjectIR],
	poolIdx krt.Index[string, ir.BackendObjectIR],
) func() {
	return func() {
		registerRouteHandlers(commonCol, cli, bcol, poolIdx)
		registerPoolHandlers(commonCol, cli, bcol)
		registerServiceHandlers(commonCol, cli, bcol)
	}
}

// registerPoolHandlers sets up handlers for InferencePool events that affect their status.
func registerPoolHandlers(
	commonCol *collections.CommonCollections,
	cli kclient.Client[*inf.InferencePool],

	bcol krt.Collection[ir.BackendObjectIR],
) {
	// Watch add/update InferencePool events
	bcol.Register(func(ev krt.Event[ir.BackendObjectIR]) {
		if ev.Event == controllers.EventDelete {
			return
		}
		updatePoolStatus(commonCol, cli, ev.Latest(), "", nil)
	})

	for _, be := range bcol.List() {
		updatePoolStatus(commonCol, cli, be, "", nil)
	}
}

// registerRouteHandlers sets up handlers for HTTPRoute events that affect InferencePools.
func registerRouteHandlers(
	commonCol *collections.CommonCollections,
	cli kclient.Client[*inf.InferencePool],
	bcol krt.Collection[ir.BackendObjectIR],
	poolIdx krt.Index[string, ir.BackendObjectIR],
) {
	// Watch add/update HTTPRoute events and trigger reconciliation for referenced pools.
	commonCol.Routes.HTTPRoutes().Register(func(ev krt.Event[ir.HttpRouteIR]) {
		reconcilePoolsForRoute(commonCol, cli, bcol, poolIdx, ev)
	})

	// Initial sweep – process routes that already existed
	for _, rt := range commonCol.Routes.HTTPRoutes().List() {
		reconcilePoolsForRoute(
			commonCol,
			cli,
			bcol,
			poolIdx,
			krt.Event[ir.HttpRouteIR]{
				Event: controllers.EventAdd,
				New:   &rt,
			},
		)
	}
}

// reconcilePoolsForRoute handles an HTTPRoute event, extracting all referenced InferencePools
// and updating their status based on the current state of the route and its parent Gateways.
func reconcilePoolsForRoute(
	commonCol *collections.CommonCollections,
	cli kclient.Client[*inf.InferencePool],
	bcol krt.Collection[ir.BackendObjectIR],
	poolIdx krt.Index[string, ir.BackendObjectIR],
	ev krt.Event[ir.HttpRouteIR],
) {
	var (
		deletedUID types.UID
		hrt        *gwv1.HTTPRoute
	)

	switch ev.Event {
	case controllers.EventAdd, controllers.EventUpdate:
		hrt = ev.New.SourceObject.(*gwv1.HTTPRoute)
	case controllers.EventDelete:
		hrt = ev.Old.SourceObject.(*gwv1.HTTPRoute)
		deletedUID = hrt.GetUID()
	default:
		return
	}

	var parentGws map[types.NamespacedName]struct{}
	if deletedUID == "" {
		parentGws = parentGateways(hrt, commonCol)
	}

	seen := map[types.NamespacedName]struct{}{}
	for _, rule := range hrt.Spec.Rules {
		for _, be := range rule.BackendRefs {
			nn := types.NamespacedName{Namespace: hrt.Namespace, Name: string(be.Name)}
			if isPoolBackend(be, nn) {
				seen[nn] = struct{}{}
			}
		}
	}

	for nn := range seen {
		if irs := poolIdx.Lookup(nn.String()); len(irs) != 0 {
			updatePoolStatus(commonCol, cli, irs[0], deletedUID, parentGws)
			continue
		}
		// If the pool is not found in the index, it may have been deleted.
		for _, ir := range bcol.List() {
			if ir.ObjectSource.Namespace == nn.Namespace && ir.ObjectSource.Name == nn.Name {
				updatePoolStatus(commonCol, cli, ir, deletedUID, parentGws)
				break
			}
		}
	}
}

// registerServiceHandlers sets up handlers for Service events that may affect InferencePools.
func registerServiceHandlers(
	commonCol *collections.CommonCollections,
	cli kclient.Client[*inf.InferencePool],
	bcol krt.Collection[ir.BackendObjectIR],
) {
	// Watch Service events and trigger reconciliation for referent InferencePools.
	commonCol.Services.Register(func(ev krt.Event[*corev1.Service]) {
		reconcilePoolsForService(commonCol, cli, bcol, ev)
	})
}

// reconcilePoolsForService validates all InferencePools that reference the given Service.
func reconcilePoolsForService(
	commonCol *collections.CommonCollections,
	cli kclient.Client[*inf.InferencePool],
	bcol krt.Collection[ir.BackendObjectIR],
	ev krt.Event[*corev1.Service],
) {
	// Pick whichever Service is non-nil
	svc := ev.Latest()
	if svc == nil && ev.Old != nil {
		svc = *ev.Old
	}
	if svc == nil {
		logger.Error("service event with no latest or old service", "event", ev.Event)
		return
	}

	svcNN := types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}
	for _, beIR := range bcol.List() {
		irPool, ok := beIR.ObjIr.(*inferencePool)
		if !ok {
			continue
		}
		if irPool.configRef.Namespace == svcNN.Namespace && irPool.configRef.Name == svcNN.Name {
			irPool.setErrors(validatePool(beIR.Obj.(*inf.InferencePool), commonCol.Services))
			updatePoolStatus(commonCol, cli, beIR, "", nil)
		}
	}
}

// isPoolBackend returns true if the given backendRef references the given InferencePool.
func isPoolBackend(be gwv1.HTTPBackendRef, poolNN types.NamespacedName) bool {
	group := inf.GroupVersion.Group
	if be.Group != nil {
		group = string(*be.Group)
	}

	kind := wellknown.InferencePoolKind
	if be.Kind != nil {
		kind = string(*be.Kind)
	}

	if be.Kind != nil {
		kind = string(*be.Kind)
	}

	if be.Namespace != nil && string(*be.Namespace) != poolNN.Namespace {
		return false
	}

	return group == inf.GroupVersion.Group &&
		kind == wellknown.InferencePoolKind &&
		be.Name == gwv1.ObjectName(poolNN.Name)
}

// referencedGateways returns all Gateways that are parents of any non-deleted
// HTTPRoute still pointing at the given pool.
func referencedGateways(
	routes []ir.HttpRouteIR, poolNN types.NamespacedName, commonCol *collections.CommonCollections,
) map[types.NamespacedName]struct{} {
	gws := make(map[types.NamespacedName]struct{})

	for _, irRt := range routes {
		rt, ok := irRt.SourceObject.(*gwv1.HTTPRoute)
		if !ok || !rt.DeletionTimestamp.IsZero() {
			continue
		}

		// Does this route reference the pool?
		poolUsed := false
		for _, rule := range rt.Spec.Rules {
			for _, be := range rule.BackendRefs {
				if isPoolBackend(be, poolNN) {
					poolUsed = true
					break
				}
			}
			if poolUsed {
				break
			}
		}
		if !poolUsed {
			continue
		}

		// Collect every Gateway parentRef on that route
		for _, pr := range rt.Spec.ParentRefs {
			if pr.Group != nil && string(*pr.Group) != gwv1.GroupName {
				// Check if this is a ListenerSet ParentRef
				if string(*pr.Group) == wellknown.XListenerSetGVK.Group &&
					pr.Kind != nil && string(*pr.Kind) == wellknown.XListenerSetKind {
					// Resolve ListenerSet to Gateways
					resolvedGws, err := resolveListenerSetGateways(pr, rt.Namespace, commonCol)
					if err != nil {
						logger.Warn("failed to resolve ListenerSet ParentRef in referencedGateways", "error", err, "listenerSet", pr.Name)
						continue
					}
					for gw := range resolvedGws {
						gws[gw] = struct{}{}
					}
				}
				continue
			}
			if pr.Kind != nil && string(*pr.Kind) != wellknown.GatewayKind {
				// Check if this is a ListenerSet ParentRef with default group
				if string(*pr.Kind) == wellknown.XListenerSetKind {
					// Resolve ListenerSet to Gateways
					resolvedGws, err := resolveListenerSetGateways(pr, rt.Namespace, commonCol)
					if err != nil {
						logger.Warn("failed to resolve ListenerSet ParentRef in referencedGateways", "error", err, "listenerSet", pr.Name)
						continue
					}
					for gw := range resolvedGws {
						gws[gw] = struct{}{}
					}
				}
				continue
			}
			ns := rt.Namespace
			if pr.Namespace != nil {
				ns = string(*pr.Namespace)
			}
			gws[types.NamespacedName{Namespace: ns, Name: string(pr.Name)}] = struct{}{}
		}
	}
	return gws
}

// resolveListenerSetGateways resolves a ListenerSet ParentRef to its constituent Gateways
func resolveListenerSetGateways(
	pr gwv1.ParentReference,
	routeNamespace string,
	commonCol *collections.CommonCollections,
) (map[types.NamespacedName]struct{}, error) {
	gws := make(map[types.NamespacedName]struct{})

	if commonCol == nil || commonCol.Client == nil {
		logger.Warn("cannot resolve ListenerSet: commonCol or client is nil", "listenerSet", pr.Name)
		return gws, nil
	}

	// Determine ListenerSet namespace
	lsNamespace := routeNamespace
	if pr.Namespace != nil {
		lsNamespace = string(*pr.Namespace)
	}

	// Create a client for XListenerSet
	lsClient := kclient.NewFiltered[*gwxv1a1.XListenerSet](
		commonCol.Client,
		kclient.Filter{ObjectFilter: commonCol.Client.ObjectFilter()},
	)

	// Get the ListenerSet object
	ls := lsClient.Get(string(pr.Name), lsNamespace)
	if ls == nil {
		return gws, fmt.Errorf("ListenerSet %s/%s not found", lsNamespace, pr.Name)
	}

	// Extract Gateway from spec.parentRef
	parentRef := ls.Spec.ParentRef
	gwNamespace := lsNamespace
	if parentRef.Namespace != nil {
		gwNamespace = string(*parentRef.Namespace)
	}
	gws[types.NamespacedName{
		Namespace: gwNamespace,
		Name:      string(parentRef.Name),
	}] = struct{}{}

	return gws, nil
}

// parentGateways returns a map of all parent Gateways referenced by the given HTTPRoute.
func parentGateways(rt *gwv1.HTTPRoute, commonCol *collections.CommonCollections) map[types.NamespacedName]struct{} {
	gws := make(map[types.NamespacedName]struct{})
	for _, pr := range rt.Spec.ParentRefs {
		if pr.Group != nil && string(*pr.Group) != gwv1.GroupName {
			// Check if this is a ListenerSet ParentRef
			if string(*pr.Group) == wellknown.XListenerSetGVK.Group &&
				pr.Kind != nil && string(*pr.Kind) == wellknown.XListenerSetKind {
				// Resolve ListenerSet to Gateways
				resolvedGws, err := resolveListenerSetGateways(pr, rt.Namespace, commonCol)
				if err != nil {
					logger.Warn("failed to resolve ListenerSet ParentRef", "error", err, "listenerSet", pr.Name)
					continue
				}
				for gw := range resolvedGws {
					gws[gw] = struct{}{}
				}
			}
			continue
		}
		if pr.Kind != nil && string(*pr.Kind) != wellknown.GatewayKind {
			// Check if this is a ListenerSet ParentRef with default group
			if string(*pr.Kind) == wellknown.XListenerSetKind {
				// Resolve ListenerSet to Gateways
				resolvedGws, err := resolveListenerSetGateways(pr, rt.Namespace, commonCol)
				if err != nil {
					logger.Warn("failed to resolve ListenerSet ParentRef", "error", err, "listenerSet", pr.Name)
					continue
				}
				for gw := range resolvedGws {
					gws[gw] = struct{}{}
				}
			}
			continue
		}
		ns := rt.Namespace
		if pr.Namespace != nil {
			ns = string(*pr.Namespace)
		}
		gws[types.NamespacedName{Namespace: ns, Name: string(pr.Name)}] = struct{}{}
	}
	return gws
}

// upsertCondition merges c into conds and returns true if that changed the conditions
// slice (new condition or any field update).
func upsert(conds *[]metav1.Condition, c metav1.Condition) {
	meta.SetStatusCondition(conds, c)
}

// updatePoolStatus reconciles status parents of an InferencePool. deletedUID != ""
// means the HTTPRoute with this UID no longer exists.
func updatePoolStatus(
	commonCol *collections.CommonCollections,
	cli kclient.Client[*inf.InferencePool],
	beIR ir.BackendObjectIR,
	deletedUID types.UID,
	parentGws map[types.NamespacedName]struct{},
) *inf.InferencePool {
	irPool, ok := beIR.ObjIr.(*inferencePool)
	if !ok {
		return nil
	}
	poolNN := types.NamespacedName{Namespace: beIR.ObjectSource.Namespace, Name: beIR.ObjectSource.Name}

	// Snapshot the errors under a lock
	errs := irPool.snapshotErrors()

	pool := cli.Get(poolNN.Name, poolNN.Namespace)
	if pool == nil {
		logger.Error("failed to get InferencePool", "ref", poolNN, "error", pluginsdk.ErrNotFound)
		return nil
	}

	allRoutes := commonCol.Routes.ListHTTPRoutesInNamespace(poolNN.Namespace)
	routes := allRoutes[:0]
	if deletedUID == "" {
		routes = append(routes, allRoutes...)
	} else {
		for _, r := range allRoutes {
			if r.SourceObject.GetUID() != deletedUID {
				routes = append(routes, r)
			}
		}
	}

	// Compute the authoritative set of Gateways that still reference the pool
	activeGws := referencedGateways(routes, poolNN, commonCol)

	for g := range parentGws {
		activeGws[g] = struct{}{}
	}

	before := append([]inf.ParentStatus(nil), pool.Status.Parents...)
	var updated []inf.ParentStatus

	updateParent := func(ref inf.ParentReference) *inf.ParentStatus {
		for i := range updated {
			if updated[i].ParentRef.Name == ref.Name &&
				updated[i].ParentRef.Namespace == ref.Namespace &&
				updated[i].ParentRef.Kind == ref.Kind {
				return &updated[i]
			}
		}
		updated = append(updated, inf.ParentStatus{ParentRef: ref})
		return &updated[len(updated)-1]
	}

	for g := range activeGws {
		p := updateParent(inf.ParentReference{
			Kind:      inf.Kind(wellknown.GatewayKind),
			Namespace: inf.Namespace(g.Namespace),
			Name:      inf.ObjectName(g.Name),
		})
		upsert(&p.Conditions, buildAcceptedCondition(pool.Generation, commonCol.ControllerName))
		upsert(&p.Conditions, buildResolvedRefsCondition(pool.Generation, errs))
	}

	if irPool.hasErrors() {
		p := updateParent(inf.ParentReference{
			Kind: inf.Kind(defaultInfPoolStatusKind),
			Name: inf.ObjectName(defaultInfPoolStatusName),
		})
		upsert(&p.Conditions, buildResolvedRefsCondition(pool.Generation, errs))
		// Per InferencePool spec, do not set Accepted on this parent
	}

	if !irPool.hasErrors() && len(activeGws) == 0 {
		cleaned := updated[:0]
		for _, p := range updated {
			if !(p.ParentRef.Kind == inf.Kind(defaultInfPoolStatusKind) &&
				p.ParentRef.Name == inf.ObjectName(defaultInfPoolStatusName)) {
				cleaned = append(cleaned, p)
			}
		}
		updated = cleaned
	}

	if parentsEqual(before, updated) {
		return pool
	}

	finalParents := append([]inf.ParentStatus(nil), updated...)

	var updatedObj *inf.InferencePool
	retryErr := retry.OnError(
		wait.Backoff{Steps: 3, Duration: 50 * time.Millisecond, Factor: 2},
		apierrors.IsConflict,
		func() error {
			cur := cli.Get(poolNN.Name, poolNN.Namespace)
			if cur == nil {
				return pluginsdk.ErrNotFound
			}

			var err error
			updatedObj, err = cli.UpdateStatus(&inf.InferencePool{
				ObjectMeta: pluginsdk.CloneObjectMetaForStatus(cur.ObjectMeta),
				Status: inf.InferencePoolStatus{
					Parents: finalParents,
				},
			})
			if apierrors.IsConflict(err) {
				logger.Debug("error updating stale status", "ref", poolNN, "error", err)
				return nil // let the conflicting Status update trigger a KRT event to requeue the updated object
			}
			return err
		})
	if retryErr != nil {
		logger.Error("failed to update InferencePool status", "pool", poolNN, "err", retryErr)
	}
	return updatedObj
}

// key returns a stable identity string for a Gateway-like ParentReference.
func key(ref inf.ParentReference) string {
	group := wellknown.InferencePoolGVK.Group
	if ref.Group != nil {
		group = string(*ref.Group)
	}
	kind := wellknown.GatewayKind
	if ref.Kind != inf.Kind(kind) {
		kind = string(ref.Kind)
	}
	ns := ""
	if ref.Namespace != inf.Namespace("") {
		ns = string(ref.Namespace)
	}
	return fmt.Sprintf("%s/%s/%s/%s", group, kind, ns, ref.Name)
}

// conditionsEqual compares two slices of metav1.Conditions without caring about order.
func conditionsEqual(a, b []metav1.Condition) bool {
	if len(a) != len(b) {
		return false
	}
	for _, ca := range a {
		cb := meta.FindStatusCondition(b, ca.Type)
		if cb == nil ||
			ca.Status != cb.Status ||
			ca.Reason != cb.Reason ||
			ca.Message != cb.Message ||
			ca.ObservedGeneration != cb.ObservedGeneration {
			return false
		}
	}
	return true
}

// parentsEqual returns true only when both the *set of parents* and every
// parent’s *Conditions* are identical.
func parentsEqual(a, b []inf.ParentStatus) bool {
	if len(a) != len(b) {
		return false
	}

	idx := make(map[string]inf.ParentStatus, len(a))
	for _, pa := range a {
		idx[key(pa.ParentRef)] = pa
	}

	for _, pb := range b {
		pa, ok := idx[key(pb.ParentRef)]
		if !ok {
			return false // parent missing
		}
		if !conditionsEqual(pa.Conditions, pb.Conditions) {
			return false // same parent, different condition set
		}
	}
	return true
}

func buildAcceptedCondition(gen int64, controllerName string) metav1.Condition {
	return metav1.Condition{
		Type:               string(inf.InferencePoolConditionAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(inf.InferencePoolReasonAccepted),
		Message:            fmt.Sprintf("InferencePool has been accepted by controller %s", controllerName),
		ObservedGeneration: gen,
		LastTransitionTime: metav1.Now(),
	}
}

func buildResolvedRefsCondition(gen int64, errs []error) metav1.Condition {
	cond := metav1.Condition{
		Type:               string(inf.InferencePoolConditionResolvedRefs),
		ObservedGeneration: gen,
		LastTransitionTime: metav1.Now(),
	}

	if len(errs) == 0 {
		cond.Status = metav1.ConditionTrue
		cond.Reason = string(inf.InferencePoolReasonResolvedRefs)
		cond.Message = "All InferencePool references have been resolved"
		return cond
	}

	var prefix string
	if len(errs) == 1 {
		prefix = "error:"
	} else {
		prefix = fmt.Sprintf("InferencePool has %d errors:", len(errs))
	}

	msgs := make([]string, 0, len(errs))
	for _, err := range errs {
		msgs = append(msgs, err.Error())
	}
	joined := strings.Join(msgs, "; ")

	cond.Status = metav1.ConditionFalse
	cond.Reason = string(inf.InferencePoolReasonInvalidExtensionRef)
	cond.Message = fmt.Sprintf("%s %s", prefix, joined)
	return cond
}
