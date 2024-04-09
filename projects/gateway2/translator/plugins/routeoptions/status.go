package routeoptions

import (
	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func (p *plugin) getPolicyReport(key types.NamespacedName) *PolicyReport {
	return p.policyStatusCache[key]
}

func (p *plugin) getOrCreatePolicyReport(routeOption *solokubev1.RouteOption) *PolicyReport {
	pr := p.getPolicyReport(client.ObjectKeyFromObject(routeOption))
	if pr == nil {
		pr = &PolicyReport{}
		pr.ObservedGeneration = routeOption.GetGeneration()
		pr.Ancestors = make(map[types.NamespacedName]*PolicyAncestorReport)
		p.policyStatusCache[client.ObjectKeyFromObject(routeOption)] = pr
	}
	return pr
}

func (pr *PolicyReport) getAncestorReport(ancestorKey types.NamespacedName) *PolicyAncestorReport {
	return pr.Ancestors[ancestorKey]
}

func (pr *PolicyReport) upsertAncestorCondition(ancestorKey types.NamespacedName, condition metav1.Condition) {
	ar := pr.getAncestorReport(ancestorKey)
	if ar == nil {
		ar = &PolicyAncestorReport{}
	}
	ar.Condition = condition
	pr.Ancestors[ancestorKey] = ar
}

// func (p *plugin) trackKubePolicyStatus(routeErrors []*validation.RouteReport_Error) {
// 	// we can coalesce route errors here to be keyed by the HTTPRoute/RouteOption object
// 	// as each HTTPRoute can only have a single attached RouteOption, we know that all gloov1.Routes
// 	// from a given HTTPRoute *should* have the same routeError
// 	for _, rerr := range routeErrors {
// 		route, ro := extractSourceKeys(rerr.GetMetadata())
// 		pr := p.getPolicyReport(ro)
// 		if pr == nil {
// 			// TODO: we got a route error for a routeoption that we weren't tracking during the plugin run; what happened?
// 		}

// 		ar := pr.getAncestorReport(route)
// 		if ar == nil {
// 			// TODO: this route error was sourced from an HTTPRoute that wasn't originally tracked; what happened?
// 		}
// 		pr.upsertAncestorCondition(route, metav1.Condition{
// 			Type:    string(gwv1alpha2.PolicyConditionAccepted),
// 			Status:  metav1.ConditionFalse,
// 			Reason:  string(gwv1alpha2.PolicyReasonInvalid),
// 			Message: rerr.GetReason(),
// 		})
// 	}
// }

func (p *plugin) setPolicyStatusAccepted(
	routeOption *solokubev1.RouteOption,
	route *gwv1.HTTPRoute,
) {
	pr := p.getOrCreatePolicyReport(routeOption)
	pr.upsertAncestorCondition(client.ObjectKeyFromObject(route), metav1.Condition{
		Type:    string(gwv1alpha2.PolicyConditionAccepted),
		Status:  metav1.ConditionTrue,
		Reason:  string(gwv1alpha2.PolicyReasonAccepted),
		Message: "Attached successfully",
	})
}

// RouteError metadata should have a single HTTPRoute & possibly RouteOption resource
// TODO: add error handling, this always assumes happy path
func extractSourceKeys(
	metadata *gloov1.SourceMetadata,
) (route, routeOption types.NamespacedName) {
	var routeRef, routeOptionRef *gloov1.SourceMetadata_SourceRef
	for _, src := range metadata.GetSources() {
		if src.GetResourceKind() == wellknown.HTTPRouteKind {
			routeRef = src
		} else if src.GetResourceKind() == sologatewayv1.RouteOptionGVK.Kind {
			routeOptionRef = src
		}
	}
	route = types.NamespacedName{
		Namespace: routeRef.GetResourceRef().GetNamespace(),
		Name:      routeRef.GetResourceRef().GetName(),
	}
	routeOption = types.NamespacedName{
		Namespace: routeOptionRef.GetResourceRef().GetNamespace(),
		Name:      routeOptionRef.GetResourceRef().GetName(),
	}
	return route, routeOption
}
