package agentgatewaysyncer

import (
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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	pluginsdkir "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// ADPRouteCollection creates the collection of translated routes
func ADPRouteCollection(
	httpRouteCol krt.Collection[*gwv1.HTTPRoute],
	grpcRouteCol krt.Collection[*gwv1.GRPCRoute],
	tcpRouteCol krt.Collection[*gwv1alpha2.TCPRoute],
	tlsRouteCol krt.Collection[*gwv1alpha2.TLSRoute],
	inputs RouteContextInputs,
	krtopts krtutil.KrtOptions,
	plugins pluginsdk.Plugin,
) krt.Collection[ADPResourcesForGateway] {
	// TODO(npolshak): look into using RouteIndex instead of raw collections to support targetRefs: https://github.com/kgateway-dev/kgateway/issues/11838
	httpRoutes := createRouteCollection(httpRouteCol, inputs, krtopts, plugins, "ADPHTTPRoutes",
		func(ctx RouteContext, obj *gwv1.HTTPRoute, rep reporter.Reporter) (RouteContext, iter.Seq2[ADPRoute, *reporter.RouteCondition]) {
			// HTTP-specific preprocessing: attach policies and setup plugins
			attachRoutePolicies(&ctx, obj)
			ctx.pluginPasses = newAgentGatewayPasses(plugins, rep, ctx.AttachedPolicies)

			route := obj.Spec
			return ctx, func(yield func(ADPRoute, *reporter.RouteCondition) bool) {
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
						res, err := convertHTTPRouteToADP(ctx, r, obj, n, idx)
						if !yield(ADPRoute{Route: res}, err) {
							return
						}
					}
				}
			}
		})

	grpcRoutes := createRouteCollection(grpcRouteCol, inputs, krtopts, plugins, "ADPGRPCRoutes",
		func(ctx RouteContext, obj *gwv1.GRPCRoute, rep reporter.Reporter) (RouteContext, iter.Seq2[ADPRoute, *reporter.RouteCondition]) {
			route := obj.Spec
			return ctx, func(yield func(ADPRoute, *reporter.RouteCondition) bool) {
				for n, r := range route.Rules {
					// Convert the entire rule with all matches at once
					res, err := convertGRPCRouteToADP(ctx, r, obj, n)
					if !yield(ADPRoute{Route: res}, err) {
						return
					}
				}
			}
		})

	tcpRoutes := createRouteCollection(tcpRouteCol, inputs, krtopts, plugins, "ADPTCPRoutes",
		func(ctx RouteContext, obj *gwv1alpha2.TCPRoute, rep reporter.Reporter) (RouteContext, iter.Seq2[ADPRoute, *reporter.RouteCondition]) {
			route := obj.Spec
			return ctx, func(yield func(ADPRoute, *reporter.RouteCondition) bool) {
				for n, r := range route.Rules {
					// Convert the entire rule with all matches at once
					res, err := convertTCPRouteToADP(ctx, r, obj, n)
					if !yield(ADPRoute{Route: res}, err) {
						return
					}
				}
			}
		})

	tlsRoutes := createRouteCollection(tlsRouteCol, inputs, krtopts, plugins, "ADPTLSRoutes",
		func(ctx RouteContext, obj *gwv1alpha2.TLSRoute, rep reporter.Reporter) (RouteContext, iter.Seq2[ADPRoute, *reporter.RouteCondition]) {
			route := obj.Spec
			return ctx, func(yield func(ADPRoute, *reporter.RouteCondition) bool) {
				for n, r := range route.Rules {
					// Convert the entire rule with all matches at once
					res, err := convertTLSRouteToADP(ctx, r, obj, n)
					if !yield(ADPRoute{Route: res}, err) {
						return
					}
				}
			}
		})

	routes := krt.JoinCollection([]krt.Collection[ADPResourcesForGateway]{httpRoutes, grpcRoutes, tcpRoutes, tlsRoutes}, krtopts.ToOptions("ADPRoutes")...)

	return routes
}

// createRouteCollection is a generic helper function that creates a KRT collection for any route type
// by extracting the common logic shared between HTTP, GRPC, TCP, and TLS route collections
func createRouteCollection[T controllers.Object](
	routeCol krt.Collection[T],
	inputs RouteContextInputs,
	krtopts krtutil.KrtOptions,
	plugins pluginsdk.Plugin,
	collectionName string,
	translator func(ctx RouteContext, obj T, rep reporter.Reporter) (RouteContext, iter.Seq2[ADPRoute, *reporter.RouteCondition]),
) krt.Collection[ADPResourcesForGateway] {
	return krt.NewManyCollection(routeCol, func(krtctx krt.HandlerContext, obj T) []ADPResourcesForGateway {
		logger.Debug("translating route", "route_name", obj.GetName(), "resource_version", obj.GetResourceVersion())

		ctx := inputs.WithCtx(krtctx)
		rm := reports.NewReportMap()
		rep := reports.NewReporter(&rm)
		routeReporter := rep.Route(obj)

		// Apply route-specific preprocessing and get the translator
		ctx, translatorSeq := translator(ctx, obj, rep)

		parentRefs, gwResult := computeRoute(ctx, obj, func(obj T) iter.Seq2[ADPRoute, *reporter.RouteCondition] {
			return translatorSeq
		})

		// gateway -> section name -> route count
		attachedRoutes := make(map[types.NamespacedName]map[string]uint)
		for _, parent := range filteredReferences(parentRefs) {
			if parent.ParentKey.Kind != wellknown.GatewayGVK {
				continue
			}
			parentGw := types.NamespacedName{
				Namespace: parent.ParentKey.Namespace,
				Name:      parent.ParentKey.Name,
			}
			if attachedRoutes[parentGw] == nil {
				attachedRoutes[parentGw] = make(map[string]uint)
			}
			attachedRoutes[parentGw][string(parent.ParentSection)]++
		}

		resourcesPerGateway := make(map[types.NamespacedName][]*api.Resource)
		for _, parent := range filteredReferences(parentRefs) {
			// Always create a route reporter entry for the parent ref
			parentRefReporter := routeReporter.ParentRef(&parent.OriginalReference)

			// for gwv1beta1 routes, build one VS per gwv1beta1+host
			routes := gwResult.routes
			if len(routes) == 0 {
				logger.Debug("no routes for parent", "route_name", obj.GetName(), "parent", parent.ParentKey)
				continue
			}
			if gwResult.error != nil {
				parentRefReporter.SetCondition(*gwResult.error)
			}

			gw := types.NamespacedName{
				Namespace: parent.ParentKey.Namespace,
				Name:      parent.ParentKey.Name,
			}
			if resourcesPerGateway[gw] == nil {
				resourcesPerGateway[gw] = make([]*api.Resource, 0)
			}
			resourcesPerGateway[gw] = append(resourcesPerGateway[gw], slices.Map(routes, func(e ADPRoute) *api.Resource {
				inner := protomarshal.Clone(e.Route)
				_, name, _ := strings.Cut(parent.InternalName, "/")
				inner.ListenerKey = name
				inner.Key = inner.GetKey() + "." + string(parent.ParentSection)
				return toADPResource(ADPRoute{Route: inner})
			})...)
		}
		var results []ADPResourcesForGateway
		for gw, res := range resourcesPerGateway {
			var attachedRoutesForGw map[string]uint
			if attachedRoutes[gw] != nil {
				attachedRoutesForGw = attachedRoutes[gw]
			}
			results = append(results, toResourceWithRoutes(gw, res, attachedRoutesForGw, rm))
		}
		return results
	}, krtopts.ToOptions(collectionName)...)
}

type conversionResult[O any] struct {
	error  *reporter.RouteCondition
	routes []O
}

// IsNil works around comparing generic types
func IsNil[O comparable](o O) bool {
	var t O
	return o == t
}

func newAgentGatewayPasses(plugs pluginsdk.Plugin,
	rep reporter.Reporter,
	aps pluginsdkir.AttachedPolicies) []pluginsdkir.AgentGatewayTranslationPass {
	var out []pluginsdkir.AgentGatewayTranslationPass
	if len(aps.Policies) == 0 {
		return out
	}
	for gk, paList := range aps.Policies {
		plugin, ok := plugs.ContributesPolicies[gk]
		if !ok || plugin.NewAgentGatewayPass == nil {
			continue
		}
		// only instantiate if there is at least one attached policy
		// OR this is the synthetic built-in GK
		if len(paList) == 0 && gk != pluginsdkir.VirtualBuiltInGK {
			continue
		}
		out = append(out, plugin.NewAgentGatewayPass(rep))
	}
	return out
}

// computeRoute holds the common route building logic shared amongst all types
func computeRoute[T controllers.Object, O comparable](ctx RouteContext, obj T, translator func(
	obj T,
) iter.Seq2[O, *reporter.RouteCondition],
) ([]routeParentReference, conversionResult[O]) {
	parentRefs := extractParentReferenceInfo(ctx, ctx.RouteParents, obj)

	convertRules := func() conversionResult[O] {
		res := conversionResult[O]{}
		for vs, err := range translator(obj) {
			// This was a hard error
			if err != nil && IsNil(vs) {
				res.error = err
				return conversionResult[O]{error: err}
			}
			// Got an error but also routes
			if err != nil {
				res.error = err
			}
			res.routes = append(res.routes, vs)
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
	pluginPasses     []pluginsdkir.AgentGatewayTranslationPass
}

type RouteContextInputs struct {
	Grants         ReferenceGrants
	RouteParents   RouteParents
	DomainSuffix   string
	Services       krt.Collection[*corev1.Service]
	InferencePools krt.Collection[*inf.InferencePool]
	Namespaces     krt.Collection[*corev1.Namespace]
	ServiceEntries krt.Collection[*networkingclient.ServiceEntry]
	Backends       *krtcollections.BackendIndex
	Policies       *krtcollections.PolicyIndex
	Plugins        pluginsdk.Plugin
}

func (i RouteContextInputs) WithCtx(krtctx krt.HandlerContext) RouteContext {
	return RouteContext{
		Krt:                krtctx,
		RouteContextInputs: i,
	}
}

type RouteWithKey struct {
	*Config
	Key string
}

func (r RouteWithKey) ResourceName() string {
	return config.NamespacedName(r.Config).String()
}

func (r RouteWithKey) Equals(o RouteWithKey) bool {
	return r.Config.Equals(o.Config)
}

// buildGatewayRoutes contains common logic to build a set of routes with gwv1beta1 semantics
func buildGatewayRoutes[T any](convertRules func() T) T {
	return convertRules()
}

// attachRoutePolicies populates ctx.AttachedPolicies with policies that
// target the given HTTPRoute. It uses the exported LookupTargetingPolicies
// from PolicyIndex.
func attachRoutePolicies(ctx *RouteContext, route *gwv1.HTTPRoute) {
	if ctx.Backends == nil {
		return
	}
	pi := ctx.Backends.PolicyIndex()
	if pi == nil {
		return
	}

	target := pluginsdkir.ObjectSource{
		Group:     wellknown.HTTPRouteGVK.Group,
		Kind:      wellknown.HTTPRouteGVK.Kind,
		Namespace: route.Namespace,
		Name:      route.Name,
	}

	pols := pi.LookupTargetingPolicies(ctx.Krt,
		pluginsdk.RouteAttachmentPoint,
		target,
		"", // route-level
		route.GetLabels())

	aps := pluginsdkir.AttachedPolicies{Policies: map[schema.GroupKind][]pluginsdkir.PolicyAtt{}}
	for _, pa := range pols {
		a := aps.Policies[pa.GroupKind]
		aps.Policies[pa.GroupKind] = append(a, pa)
	}

	if _, ok := aps.Policies[pluginsdkir.VirtualBuiltInGK]; !ok {
		aps.Policies[pluginsdkir.VirtualBuiltInGK] = nil
	}
	ctx.AttachedPolicies = aps
}
