package proxy_syncer

import (
	"fmt"
	"slices"
	"strings"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// Policy status is otherwise produced by reverse lookup: Gateway and Backend translation ask
// the policy index "which policies target me" and report an ancestor per match. A policy whose
// targetRef names an object that does not exist never matches anything, so nothing reports on
// it and its status stays empty (kgateway-dev/kgateway#11160, #11624). Worse, a policy with one
// valid and one misspelled targetRef looks fully healthy.
//
// This producer walks each policy's own targetRefs forward instead. Every explicit ref is
// resolved against the informer-backed collection for its kind through krt, so the check is
// dependency tracked and costs no API calls. Unresolved refs are reported on one ancestor per
// policy, PolicyTargetsAncestorRef, with Accepted=False/TargetNotFound, alongside whatever
// Gateway ancestors the valid refs produced. When the target appears, the contribution stops
// and the writer retracts the ancestor through the normal stale-status path.

// policyTargetResolver checks that the object one explicit targetRef names exists and, when
// the ref carries a sectionName, that the section exists on it. The error is user-facing and
// becomes the status message.
type policyTargetResolver func(kctx krt.HandlerContext, namespace, name, sectionName string) error

// policyTargetResolvers maps a target GroupKind to its resolver. Kinds without an entry are
// not checked: a policy targeting them reports nothing extra, as before.
type policyTargetResolvers map[schema.GroupKind]policyTargetResolver

// newPolicyTargetResolvers builds resolvers for every kind kgateway's policies may target and
// has an informer for. Collections a CommonCollections was built without are skipped, so a
// partially initialized set (as some tests build) simply checks fewer kinds.
func newPolicyTargetResolvers(commonCols *collections.CommonCollections, backends krt.Collection[*kgateway.Backend]) policyTargetResolvers {
	resolvers := policyTargetResolvers{}
	if commonCols == nil {
		return resolvers
	}
	if commonCols.RawGateways != nil {
		resolvers[wellknown.GatewayGVK.GroupKind()] = sectionedTargetResolver(commonCols.RawGateways, wellknown.GatewayGVK.Kind,
			func(gw *gwv1.Gateway, section string) bool {
				return slices.ContainsFunc(gw.Spec.Listeners, func(l gwv1.Listener) bool { return string(l.Name) == section })
			})
	}
	if commonCols.RawListenerSets != nil {
		// The normalized collection holds both the promoted and legacy flavors keyed by
		// namespace/name, so either target kind resolves against it.
		for _, gvk := range wellknown.AllListenerSetGVKs() {
			resolvers[gvk.GroupKind()] = sectionedTargetResolver(commonCols.RawListenerSets, gvk.Kind,
				func(ls *gwv1.ListenerSet, section string) bool {
					return slices.ContainsFunc(ls.Spec.Listeners, func(l gwv1.ListenerEntry) bool { return string(l.Name) == section })
				})
		}
	}
	if commonCols.RawHTTPRoutes != nil {
		resolvers[wellknown.HTTPRouteGVK.GroupKind()] = sectionedTargetResolver(commonCols.RawHTTPRoutes, wellknown.HTTPRouteGVK.Kind,
			func(route *gwv1.HTTPRoute, section string) bool {
				return slices.ContainsFunc(route.Spec.Rules, func(r gwv1.HTTPRouteRule) bool {
					return r.Name != nil && string(*r.Name) == section
				})
			})
	}
	if commonCols.RawGRPCRoutes != nil {
		resolvers[wellknown.GRPCRouteGVK.GroupKind()] = sectionedTargetResolver(commonCols.RawGRPCRoutes, wellknown.GRPCRouteGVK.Kind,
			func(route *gwv1.GRPCRoute, section string) bool {
				return slices.ContainsFunc(route.Spec.Rules, func(r gwv1.GRPCRouteRule) bool {
					return r.Name != nil && string(*r.Name) == section
				})
			})
	}
	// Service and Backend refs may carry a sectionName (a port name) on some policy kinds;
	// only the object's existence is checked for them.
	if commonCols.Services != nil {
		resolvers[wellknown.ServiceGVK.GroupKind()] = existingTargetResolver(commonCols.Services, wellknown.ServiceGVK.Kind)
	}
	if backends != nil {
		resolvers[wellknown.BackendGVK.GroupKind()] = existingTargetResolver(backends, wellknown.BackendGVK.Kind)
	}
	return resolvers
}

// existingTargetResolver checks only that the named object exists.
func existingTargetResolver[T controllers.Object](col krt.Collection[T], kind string) policyTargetResolver {
	return func(kctx krt.HandlerContext, namespace, name, _ string) error {
		if krt.FetchOne(kctx, col, krt.FilterKey(namespace+"/"+name)) == nil {
			return targetNotFoundError(kind, namespace, name)
		}
		return nil
	}
}

// sectionedTargetResolver checks that the named object exists and, if the ref is scoped to a
// section, that hasSection finds it on the object.
func sectionedTargetResolver[T controllers.Object](col krt.Collection[T], kind string, hasSection func(obj T, section string) bool) policyTargetResolver {
	return func(kctx krt.HandlerContext, namespace, name, sectionName string) error {
		obj := krt.FetchOne(kctx, col, krt.FilterKey(namespace+"/"+name))
		if obj == nil {
			return targetNotFoundError(kind, namespace, name)
		}
		if sectionName != "" && !hasSection(*obj, sectionName) {
			return fmt.Errorf("sectionName %q not found in %s %s/%s", sectionName, kind, namespace, name)
		}
		return nil
	}
}

func targetNotFoundError(kind, namespace, name string) error {
	return fmt.Errorf("%s %s/%s not found", kind, namespace, name)
}

// PolicyTargetsAncestorRef is the ancestor under which a policy's targetRef resolution is
// reported: the policy itself. A missing target has no Gateway to report under, and using the
// missing ref as the ancestor would collide with the real ancestor the moment the target
// appears (Backend-attached policies report the target object as their ancestor) and would
// eat into the ancestor cap one entry per typo. One self-referencing entry per policy avoids
// both. Group and kind are explicit because the CRD schema defaults an ancestorRef's kind to
// Gateway when they are omitted.
func PolicyTargetsAncestorRef(policy ir.ObjectSource) gwv1.ParentReference {
	return gwv1.ParentReference{
		Group:     new(gwv1.Group(policy.Group)),
		Kind:      new(gwv1.Kind(policy.Kind)),
		Namespace: new(gwv1.Namespace(policy.Namespace)),
		Name:      gwv1.ObjectName(policy.Name),
	}
}

// policyTargetStatusContributions emits one contribution per policy that has at least one
// explicit targetRef the resolvers cannot resolve, and nothing for every other policy.
func policyTargetStatusContributions(
	policies krt.Collection[ir.PolicyWrapper],
	resolvers policyTargetResolvers,
	krtopts krtutil.KrtOptions,
	name string,
) krt.Collection[reports.StatusContribution] {
	return krt.NewCollection(policies, func(kctx krt.HandlerContext, policy ir.PolicyWrapper) *reports.StatusContribution {
		problems := unresolvedPolicyTargets(kctx, policy, resolvers)
		if len(problems) == 0 {
			return nil
		}
		reportMap := buildPolicyTargetReport(policy, problems)
		contributions := reports.StatusContributionsFromReportMap(reports.StatusSource{
			Kind: reports.PolicyTargetStatusSource,
			Name: policy.ResourceName(),
		}, reportMap)
		return &contributions[0]
	}, krtopts.ToOptions(name+"-policyTargetStatusContributions")...)
}

// unresolvedPolicyTargets returns one message per explicit targetRef that does not resolve, in
// targetRefs order. Label selectors (refs without a Name) and kinds without a resolver are
// skipped: a selector matching nothing is a valid state, not a missing target.
func unresolvedPolicyTargets(kctx krt.HandlerContext, policy ir.PolicyWrapper, resolvers policyTargetResolvers) []string {
	var problems []string
	for _, ref := range policy.TargetRefs {
		if ref.Name == "" {
			continue
		}
		resolve, ok := resolvers[schema.GroupKind{Group: ref.Group, Kind: ref.Kind}]
		if !ok {
			continue
		}
		if err := resolve(kctx, policy.Namespace, ref.Name, ref.SectionName); err != nil {
			problems = append(problems, err.Error())
		}
	}
	return problems
}

// buildPolicyTargetReport reports the unresolved targets on the policy's own ancestor.
func buildPolicyTargetReport(policy ir.PolicyWrapper, problems []string) reports.ReportMap {
	reportMap := reports.NewPolicyReportMap()
	var generation int64
	if policy.Policy != nil {
		generation = policy.Policy.GetGeneration()
	}
	key := reporter.PolicyKey{
		Group:     policy.Group,
		Kind:      policy.Kind,
		Namespace: policy.Namespace,
		Name:      policy.Name,
	}
	ancestor := reports.NewReporter(&reportMap).Policy(key, generation).AncestorRef(PolicyTargetsAncestorRef(policy.ObjectSource))
	ancestor.SetCondition(reporter.PolicyCondition{
		Type:    string(shared.PolicyConditionAccepted),
		Status:  metav1.ConditionFalse,
		Reason:  string(shared.PolicyReasonTargetNotFound),
		Message: strings.Join(problems, "; "),
	})
	ancestor.SetCondition(reporter.PolicyCondition{
		Type:    string(shared.PolicyConditionAttached),
		Status:  metav1.ConditionFalse,
		Reason:  string(shared.PolicyReasonTargetNotFound),
		Message: reporter.PolicyTargetNotFoundMsg,
	})
	return reportMap
}
