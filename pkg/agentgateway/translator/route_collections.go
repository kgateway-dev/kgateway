package translator

import (
	"fmt"
	"iter"
	"strings"

	"github.com/agentgateway/agentgateway/go/api"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/slices"
	"istio.io/istio/pkg/util/protomarshal"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	agwir "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/ir"
	pluginsdkir "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// AgwRouteCollection creates the collection of translated Routes
func AgwRouteCollection(
	httpRouteCol krt.Collection[*gwv1.HTTPRoute],
	grpcRouteCol krt.Collection[*gwv1.GRPCRoute],
	tcpRouteCol krt.Collection[*gwv1alpha2.TCPRoute],
	tlsRouteCol krt.Collection[*gwv1alpha2.TLSRoute],
	inputs RouteContextInputs,
	krtopts krtutil.KrtOptions,
) krt.Collection[agwir.AgwResourcesForGateway] {
	httpRoutes := createRouteCollection(httpRouteCol, inputs, krtopts, "AgwHTTPRoutes",
		func(ctx RouteContext, obj *gwv1.HTTPRoute, rep reporter.Reporter) (RouteContext, iter.Seq2[AgwRoute, *reporter.RouteCondition]) {
			route := obj.Spec
			return ctx, func(yield func(AgwRoute, *reporter.RouteCondition) bool) {
				for n, r := range route.Rules {
					// split the rule to make sure each rule has up to one match
					matches := slices.Reference(r.Matches)
					if len(matches) == 0 {
						matches = append(matches, nil)
					}
					for idx, m := range matches {
						if m != nil {
							r.Matches = []gwv1.HTTPRouteMatch{*m}
						}
						res, err := ConvertHTTPRouteToAgw(ctx, r, obj, n, idx)
						if !yield(AgwRoute{Route: res}, err) {
							return
						}
					}
				}
			}
		})

	grpcRoutes := createRouteCollection(grpcRouteCol, inputs, krtopts, "AgwGRPCRoutes",
		func(ctx RouteContext, obj *gwv1.GRPCRoute, rep reporter.Reporter) (RouteContext, iter.Seq2[AgwRoute, *reporter.RouteCondition]) {
			route := obj.Spec
			return ctx, func(yield func(AgwRoute, *reporter.RouteCondition) bool) {
				for n, r := range route.Rules {
					// Convert the entire rule with all matches at once
					res, err := ConvertGRPCRouteToAgw(ctx, r, obj, n)
					if !yield(AgwRoute{Route: res}, err) {
						return
					}
				}
			}
		})

	tcpRoutes := createTCPRouteCollection(tcpRouteCol, inputs, krtopts, "AgwTCPRoutes",
		func(ctx RouteContext, obj *gwv1alpha2.TCPRoute, rep reporter.Reporter) (RouteContext, iter.Seq2[AgwTCPRoute, *reporter.RouteCondition]) {
			route := obj.Spec
			return ctx, func(yield func(AgwTCPRoute, *reporter.RouteCondition) bool) {
				for n, r := range route.Rules {
					// Convert the entire rule with all matches at once
					res, err := ConvertTCPRouteToAgw(ctx, r, obj, n)
					if !yield(AgwTCPRoute{TCPRoute: res}, err) {
						return
					}
				}
			}
		})

	tlsRoutes := createTCPRouteCollection(tlsRouteCol, inputs, krtopts, "AgwTLSRoutes",
		func(ctx RouteContext, obj *gwv1alpha2.TLSRoute, rep reporter.Reporter) (RouteContext, iter.Seq2[AgwTCPRoute, *reporter.RouteCondition]) {
			route := obj.Spec
			return ctx, func(yield func(AgwTCPRoute, *reporter.RouteCondition) bool) {
				for n, r := range route.Rules {
					// Convert the entire rule with all matches at once
					res, err := ConvertTLSRouteToAgw(ctx, r, obj, n)
					if !yield(AgwTCPRoute{TCPRoute: res}, err) {
						return
					}
				}
			}
		})

	routes := krt.JoinCollection([]krt.Collection[agwir.AgwResourcesForGateway]{httpRoutes, grpcRoutes, tcpRoutes, tlsRoutes}, krtopts.ToOptions("AgwRoutes")...)

	return routes
}

// ProcessParentReferences processes filtered parent references and builds resources per gateway.
// It emits exactly one ParentStatus per Gateway (aggregate across listeners).
// If no listeners are allowed, the Accepted reason is:
//   - NotAllowedByListeners  => when the parent Gateway is cross-namespace w.r.t. the route
//   - NoMatchingListenerHostname => otherwise
func ProcessParentReferences[T any](
	parentRefs []RouteParentReference,
	gwResult ConversionResult[T],
	routeNN types.NamespacedName, // <-- route namespace/name so we can detect cross-NS parents
	routeReporter reporter.RouteReporter,
	resourceMapper func(T, RouteParentReference) *api.Resource,
) map[types.NamespacedName][]*api.Resource {
	resourcesPerGateway := make(map[types.NamespacedName][]*api.Resource)

	// Build the "allowed" set from FilteredReferences (listener-scoped).
	allowed := make(map[string]struct{})
	for _, p := range FilteredReferences(parentRefs) {
		if p.ParentKey.Kind != wellknown.GatewayGVK {
			continue
		}
		k := fmt.Sprintf("%s/%s/%s/%s", p.ParentKey.Namespace, p.ParentKey.Name, p.ParentKey.Kind, string(p.ParentSection))
		allowed[k] = struct{}{}
	}

	// Aggregate per Gateway for status; also track whether any raw parent was cross-namespace.
	type gwAgg struct {
		anyAllowed bool
		rep        RouteParentReference
	}
	agg := make(map[types.NamespacedName]*gwAgg)
	crossNS := make(map[types.NamespacedName]bool)

	for _, p := range parentRefs {
		if p.ParentKey.Kind != wellknown.GatewayGVK {
			continue
		}
		gwNN := types.NamespacedName{Namespace: p.ParentKey.Namespace, Name: p.ParentKey.Name}
		if _, ok := agg[gwNN]; !ok {
			agg[gwNN] = &gwAgg{anyAllowed: false, rep: p}
		}
		if p.ParentKey.Namespace != routeNN.Namespace {
			crossNS[gwNN] = true
		}
	}

	// If conversion (backend/filter resolution) failed, ResolvedRefs=False for all parents.
	resolvedOK := (gwResult.Error == nil)

	// Consider each raw parentRef (listener-scoped) for mapping.
	for _, parent := range parentRefs {
		gwNN := types.NamespacedName{Namespace: parent.ParentKey.Namespace, Name: parent.ParentKey.Name}
		listener := string(parent.ParentSection)
		keyStr := fmt.Sprintf("%s/%s/%s/%s", parent.ParentKey.Namespace, parent.ParentKey.Name, parent.ParentKey.Kind, listener)
		_, isAllowed := allowed[keyStr]

		if isAllowed {
			if a := agg[gwNN]; a != nil {
				a.anyAllowed = true
			}
		}
		// Only attach resources when listener is allowed. Even if ResolvedRefs is false,
		// we still attach so any DirectResponse policy can return 5xx as required.
		if !isAllowed {
			continue
		}
		var mapped []*api.Resource
		routes := gwResult.Routes
		mapped = make([]*api.Resource, 0, len(routes))
		for i := range routes {
			if r := resourceMapper(routes[i], parent); r != nil {
				mapped = append(mapped, r)
			}
		}
		resourcesPerGateway[gwNN] = append(resourcesPerGateway[gwNN], mapped...)
	}

	// Emit exactly ONE ParentStatus per Gateway (aggregate across listeners; no SectionName).
	for gwNN, a := range agg {
		parent := a.rep
		prStatusRef := parent.OriginalReference
		{
			stringPtr := func(s string) *string { return &s }
			prStatusRef.Group = (*gwv1.Group)(stringPtr(wellknown.GatewayGVK.Group))
			prStatusRef.Kind = (*gwv1.Kind)(stringPtr(wellknown.GatewayGVK.Kind))
			prStatusRef.Namespace = (*gwv1.Namespace)(stringPtr(parent.ParentKey.Namespace))
			prStatusRef.Name = gwv1.ObjectName(parent.ParentKey.Name)
			prStatusRef.SectionName = nil
		}
		pr := routeReporter.ParentRef(&prStatusRef)
		resolvedReason := reasonResolvedRefs(gwResult.Error, resolvedOK)

		if a.anyAllowed {
			pr.SetCondition(reporter.RouteCondition{
				Type:   gwv1.RouteConditionAccepted,
				Status: metav1.ConditionTrue,
				Reason: gwv1.RouteReasonAccepted,
			})
		} else {
			// Nothing attached: choose reason based on *why* it wasn't allowed.
			// Priority:
			// 1) Cross-namespace and listeners don’t allow it -> NotAllowedByListeners
			// 2) sectionName specified but no such listener on the parent -> NoMatchingParent
			// 3) Otherwise, no hostname intersection -> NoMatchingListenerHostname
			reason := gwv1.RouteConditionReason("NoMatchingListenerHostname")
			msg := "No route hostnames intersect any listener hostname"
			if crossNS[gwNN] {
				reason = gwv1.RouteReasonNotAllowedByListeners
				msg = "Parent listener not usable or not permitted"
			} else if a.rep.OriginalReference.SectionName != nil {
				// Use string literal to avoid compile issues if the constant name differs.
				reason = gwv1.RouteConditionReason("NoMatchingParent")
				msg = "No listener with the specified sectionName on the parent Gateway"
			}
			pr.SetCondition(reporter.RouteCondition{
				Type:    gwv1.RouteConditionAccepted,
				Status:  metav1.ConditionFalse,
				Reason:  reason,
				Message: msg,
			})
		}

		pr.SetCondition(reporter.RouteCondition{
			Type: gwv1.RouteConditionResolvedRefs,
			Status: func() metav1.ConditionStatus {
				if resolvedOK {
					return metav1.ConditionTrue
				}
				return metav1.ConditionFalse
			}(),
			Reason: resolvedReason,
			Message: func() string {
				if gwResult.Error != nil {
					return gwResult.Error.Message
				}
				return ""
			}(),
		})
	}
	return resourcesPerGateway
}

// reasonResolvedRefs picks a ResolvedRefs reason from a conversion failure condition.
// Falls back to "ResolvedRefs" (when ok) or "Invalid" (when not ok and no specific reason).
func reasonResolvedRefs(cond *reporter.RouteCondition, ok bool) gwv1.RouteConditionReason {
	if ok {
		return gwv1.RouteReasonResolvedRefs
	}
	if cond != nil && cond.Reason != "" {
		return cond.Reason
	}
	return gwv1.RouteConditionReason("Invalid")
}

// buildAttachedRoutesMapAllowed is the same as buildAttachedRoutesMap,
// but only for already-evaluated, allowed parentRefs.
func buildAttachedRoutesMapAllowed(
	allowedParents []RouteParentReference,
	routeNN types.NamespacedName,
) map[types.NamespacedName]map[string]uint {
	attached := make(map[types.NamespacedName]map[string]uint)
	type attachKey struct {
		gw       types.NamespacedName
		listener string
		route    types.NamespacedName
	}
	seen := make(map[attachKey]struct{})

	for _, parent := range allowedParents {
		if parent.ParentKey.Kind != wellknown.GatewayGVK {
			continue
		}
		gw := types.NamespacedName{Namespace: parent.ParentKey.Namespace, Name: parent.ParentKey.Name}
		lis := string(parent.ParentSection)

		k := attachKey{gw: gw, listener: lis, route: routeNN}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}

		if attached[gw] == nil {
			attached[gw] = make(map[string]uint)
		}
		attached[gw][lis]++
	}
	return attached
}

// Generic function that handles the common logic
func createRouteCollectionGeneric[T controllers.Object, R comparable](
	routeCol krt.Collection[T],
	inputs RouteContextInputs,
	krtopts krtutil.KrtOptions,
	collectionName string,
	translator func(ctx RouteContext, obj T, rep reporter.Reporter) (RouteContext, iter.Seq2[R, *reporter.RouteCondition]),
	resourceTransformer func(route R, parent RouteParentReference) *api.Resource,
) krt.Collection[agwir.AgwResourcesForGateway] {
	return krt.NewManyCollection(routeCol, func(krtctx krt.HandlerContext, obj T) []agwir.AgwResourcesForGateway {
		logger.Debug("translating route", "route_name", obj.GetName(), "resource_version", obj.GetResourceVersion())

		ctx := inputs.WithCtx(krtctx)
		rm := reports.NewReportMap()
		rep := reports.NewReporter(&rm)
		routeReporter := rep.Route(obj)

		// Apply route-specific preprocessing and get the translator
		ctx, translatorSeq := translator(ctx, obj, rep)

		parentRefs, gwResult := computeRoute(ctx, obj, func(obj T) iter.Seq2[R, *reporter.RouteCondition] {
			return translatorSeq
		})

		// gateway -> section name -> route count
		routeNN := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}
		ln := ListenersPerGateway(parentRefs)
		allowedParents := FilteredReferences(parentRefs)
		attachedRoutes := buildAttachedRoutesMapAllowed(allowedParents, routeNN)
		EnsureZeroes(attachedRoutes, ln)

		resourcesPerGateway := ProcessParentReferences[R](
			parentRefs,
			gwResult,
			routeNN,
			routeReporter,
			resourceTransformer,
		)

		var results []agwir.AgwResourcesForGateway
		allRelevantGateways := make(map[types.NamespacedName]struct{})

		// Collect all relevant gateways
		for gw := range resourcesPerGateway {
			allRelevantGateways[gw] = struct{}{}
		}
		for gw := range attachedRoutes {
			allRelevantGateways[gw] = struct{}{}
		}

		for gw := range allRelevantGateways {
			var resources []*api.Resource
			var routeCounts map[string]uint

			if res, hasResources := resourcesPerGateway[gw]; hasResources {
				resources = res
			}
			if ar, hasRoutes := attachedRoutes[gw]; hasRoutes {
				routeCounts = ar
			}

			results = append(results, ToResourceWithRoutes(gw, resources, routeCounts, rm))
		}
		return results
	}, krtopts.ToOptions(collectionName)...)
}

// Simplified HTTP route collection function
func createRouteCollection[T controllers.Object](
	routeCol krt.Collection[T],
	inputs RouteContextInputs,
	krtopts krtutil.KrtOptions,
	collectionName string,
	translator func(ctx RouteContext, obj T, rep reporter.Reporter) (RouteContext, iter.Seq2[AgwRoute, *reporter.RouteCondition]),
) krt.Collection[agwir.AgwResourcesForGateway] {
	return createRouteCollectionGeneric(
		routeCol,
		inputs,
		krtopts,
		collectionName,
		translator,
		func(e AgwRoute, parent RouteParentReference) *api.Resource {
			inner := protomarshal.Clone(e.Route)
			_, name, _ := strings.Cut(parent.InternalName, "/")
			inner.ListenerKey = name
			if sec := string(parent.ParentSection); sec != "" {
				inner.Key = inner.GetKey() + "." + sec
			} else {
				inner.Key = inner.GetKey()
			}
			return ToAgwResource(AgwRoute{Route: inner})
		},
	)
}

// Simplified TCP route collection function (plugins parameter removed)
func createTCPRouteCollection[T controllers.Object](
	routeCol krt.Collection[T],
	inputs RouteContextInputs,
	krtopts krtutil.KrtOptions,
	collectionName string,
	translator func(ctx RouteContext, obj T, rep reporter.Reporter) (RouteContext, iter.Seq2[AgwTCPRoute, *reporter.RouteCondition]),
) krt.Collection[agwir.AgwResourcesForGateway] {
	return createRouteCollectionGeneric(
		routeCol,
		inputs,
		krtopts,
		collectionName,
		translator,
		func(e AgwTCPRoute, parent RouteParentReference) *api.Resource {
			// TCP route wrapper doesn't expose a `Route` field like HTTP.
			// For TCP we don't mutate ListenerKey/Key here; just pass through.
			return ToAgwResource(e)
		},
	)
}

// ListenersPerGateway returns the set of listener sectionNames referenced for each parent Gateway,
// regardless of whether they are allowed.
func ListenersPerGateway(parentRefs []RouteParentReference) map[types.NamespacedName]map[string]struct{} {
	l := make(map[types.NamespacedName]map[string]struct{})
	for _, p := range parentRefs {
		if p.ParentKey.Kind != wellknown.GatewayGVK {
			continue
		}
		gw := types.NamespacedName{Namespace: p.ParentKey.Namespace, Name: p.ParentKey.Name}
		if l[gw] == nil {
			l[gw] = make(map[string]struct{})
		}
		l[gw][string(p.ParentSection)] = struct{}{}
	}
	return l
}

// EnsureZeroes pre-populates AttachedRoutes with explicit 0 entries for every referenced listener,
// so writers that "replace" rather than "merge" will correctly set zero.
func EnsureZeroes(
	attached map[types.NamespacedName]map[string]uint,
	ln map[types.NamespacedName]map[string]struct{},
) {
	for gw, set := range ln {
		if attached[gw] == nil {
			attached[gw] = make(map[string]uint)
		}
		for lis := range set {
			if _, ok := attached[gw][lis]; !ok {
				attached[gw][lis] = 0
			}
		}
	}
}

type ConversionResult[O any] struct {
	Error  *reporter.RouteCondition
	Routes []O
}

// IsNil works around comparing generic types
func IsNil[O comparable](o O) bool {
	var t O
	return o == t
}

// computeRoute holds the common route building logic shared amongst all types
func computeRoute[T controllers.Object, O comparable](ctx RouteContext, obj T, translator func(
	obj T,
) iter.Seq2[O, *reporter.RouteCondition],
) ([]RouteParentReference, ConversionResult[O]) {
	parentRefs := extractParentReferenceInfo(ctx, ctx.RouteParents, obj)

	convertRules := func() ConversionResult[O] {
		res := ConversionResult[O]{}
		for vs, err := range translator(obj) {
			// This was a hard Error
			if err != nil && IsNil(vs) {
				res.Error = err
				return ConversionResult[O]{Error: err}
			}
			// Got an error but also Routes
			if err != nil {
				res.Error = err
			}
			res.Routes = append(res.Routes, vs)
		}
		return res
	}
	gwResult := buildGatewayRoutes(convertRules)

	return parentRefs, gwResult
}

// RouteContext defines a common set of inputs to a route collection for agentgateway.
// This should be built once per route translation and not shared outside of that.
// The embedded RouteContextInputs is typically based into a collection, then translated to a RouteContext with RouteContextInputs.WithCtx().
type RouteContext struct {
	Krt krt.HandlerContext
	RouteContextInputs
	AttachedPolicies pluginsdkir.AttachedPolicies
	pluginPasses     []agwir.AgwTranslationPass
}

// RouteContextInputs defines the collections needed to translate a route.
type RouteContextInputs struct {
	Grants          ReferenceGrants
	RouteParents    RouteParents
	Services        krt.Collection[*corev1.Service]
	InferencePools  krt.Collection[*inf.InferencePool]
	Namespaces      krt.Collection[*corev1.Namespace]
	ServiceEntries  krt.Collection[*networkingclient.ServiceEntry]
	Backends        krt.Collection[*v1alpha1.Backend]
	Policies        *krtcollections.PolicyIndex
	DirectResponses krt.Collection[*v1alpha1.DirectResponse]
}

func (i RouteContextInputs) WithCtx(krtctx krt.HandlerContext) RouteContext {
	return RouteContext{
		Krt:                krtctx,
		RouteContextInputs: i,
	}
}

// RouteWithKey is a wrapper for a Route
type RouteWithKey struct {
	*Config
}

func (r RouteWithKey) ResourceName() string {
	return config.NamespacedName(r.Config).String()
}

func (r RouteWithKey) Equals(o RouteWithKey) bool {
	return r.Config.Equals(o.Config)
}

// buildGatewayRoutes contains common logic to build a set of Routes with v1/alpha2 semantics
func buildGatewayRoutes[T any](convertRules func() T) T {
	return convertRules()
}
