package proxy_syncer

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	utilkrt "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

// Policy status is otherwise produced by reverse lookup: Gateway and Backend translation ask
// the policy index "which policies target me" and report an ancestor per match. A policy whose
// targetRef names an object that does not exist never matches anything, so nothing reports on
// it and its status stays empty (kgateway-dev/kgateway#11160, #11624). Worse, a policy with one
// valid and one misspelled targetRef looks fully healthy.
//
// This producer walks each policy's own targetRefs forward instead. Every explicit ref is
// resolved against the informer-backed collection for its kind through krt, so the check is
// dependency tracked and costs no API calls. Unresolved refs are reported on the policy's
// synthetic summary ancestor (reporter.PolicyStatusSummaryAncestorRef) with
// Accepted=False/TargetNotFound, alongside whatever Gateway ancestors the valid refs produced.
// Using the missing ref itself as the ancestor would instead publish a ref to an object that
// does not exist and cost one ancestor per typo under the ancestor cap. When the target appears, the contribution stops
// and the writer retracts the ancestor through the normal stale-status path.

// PolicyTargetResolver checks that the object one explicit targetRef names exists and, when
// the ref carries a sectionName, that the section exists on it. namespace is the policy's own,
// since targetRefs are namespace local. The error is user-facing and becomes the status
// message. Resolvers run inside a krt transform: they must read through kctx so the check
// re-runs when the target appears or disappears.
type PolicyTargetResolver func(kctx krt.HandlerContext, namespace, name, sectionName string) error

// policyTargetResolvers maps a target GroupKind to its resolver. Kinds without an entry are
// not checked: a policy targeting them reports nothing extra, as before.
type policyTargetResolvers map[schema.GroupKind]PolicyTargetResolver

// newPolicyTargetResolvers builds resolvers for every kind kgateway's policies may target and
// has an informer for: the Gateway API kinds, Service, Backend, and every alias kind a backend
// plugin declares (for example the Istio Hostname and ServiceEntry aliases). Collections a
// CommonCollections was built without are skipped, so a partially initialized set (as some
// tests build) simply checks fewer kinds. extra, registered through WithPolicyTargetResolver,
// is applied last and so replaces a built-in resolver for the same kind.
func newPolicyTargetResolvers(
	commonCols *collections.CommonCollections,
	backendPlugins map[schema.GroupKind]sdk.BackendPlugin,
	extra policyTargetResolvers,
) policyTargetResolvers {
	resolvers := policyTargetResolvers{}
	if commonCols == nil {
		maps.Copy(resolvers, extra)
		return resolvers
	}
	if commonCols.RawGateways != nil {
		resolvers[wellknown.GatewayGVK.GroupKind()] = objectTargetResolver(commonCols.RawGateways, wellknown.GatewayGVK.Kind, nil,
			func(gw *gwv1.Gateway) []string {
				return namesOf(gw.Spec.Listeners, func(l gwv1.Listener) string { return string(l.Name) })
			})
	}
	if commonCols.RawListenerSets != nil {
		// The normalized collection holds both the promoted and the legacy XListenerSet objects
		// keyed by namespace/name, with the source GVK kept in TypeMeta (empty means promoted, as
		// on the attachment path). Attachment keys policies on that GVK, so a same-named object
		// of the other flavor is not the target and must not count as found.
		for _, gvk := range wellknown.AllListenerSetGVKs() {
			resolvers[gvk.GroupKind()] = objectTargetResolver(commonCols.RawListenerSets, gvk.Kind,
				func(ls *gwv1.ListenerSet) bool {
					return statussync.ObjectGVKOrDefault(ls, wellknown.ListenerSetGVK).GroupKind() == gvk.GroupKind()
				},
				func(ls *gwv1.ListenerSet) []string {
					return namesOf(ls.Spec.Listeners, func(l gwv1.ListenerEntry) string { return string(l.Name) })
				})
		}
	}
	if commonCols.RawHTTPRoutes != nil {
		resolvers[wellknown.HTTPRouteGVK.GroupKind()] = objectTargetResolver(commonCols.RawHTTPRoutes, wellknown.HTTPRouteGVK.Kind, nil,
			func(route *gwv1.HTTPRoute) []string {
				return namesOf(route.Spec.Rules, func(r gwv1.HTTPRouteRule) string { return string(ptr.Deref(r.Name, "")) })
			})
	}
	if commonCols.RawGRPCRoutes != nil {
		resolvers[wellknown.GRPCRouteGVK.GroupKind()] = objectTargetResolver(commonCols.RawGRPCRoutes, wellknown.GRPCRouteGVK.Kind, nil,
			func(route *gwv1.GRPCRoute) []string {
				return namesOf(route.Spec.Rules, func(r gwv1.GRPCRouteRule) string { return string(ptr.Deref(r.Name, "")) })
			})
	}
	// TCPRoutes and TLSRoutes attach policies only at the route level, never per rule, so their
	// sectionName is left unchecked, as for Service below. Every served version is normalized
	// into one collection, and the group and kind are the same across versions.
	if commonCols.RawTCPRoutes != nil {
		resolvers[wellknown.TCPRouteGVK.GroupKind()] = objectTargetResolver(commonCols.RawTCPRoutes, wellknown.TCPRouteGVK.Kind, nil, nil)
	}
	if commonCols.RawTLSRoutes != nil {
		resolvers[wellknown.TLSRouteGVK.GroupKind()] = objectTargetResolver(commonCols.RawTLSRoutes, wellknown.TLSRouteGVK.Kind, nil, nil)
	}
	// Service and Backend refs may carry a sectionName (a port name) on some policy kinds;
	// only the object's existence is checked for them.
	if commonCols.Services != nil {
		resolvers[wellknown.ServiceGVK.GroupKind()] = objectTargetResolver(commonCols.Services, wellknown.ServiceGVK.Kind, nil, nil)
	}
	if backends := backendPlugins[wellknown.BackendGVK.GroupKind()].RawBackends; backends != nil {
		resolvers[wellknown.BackendGVK.GroupKind()] = objectTargetResolver(backends, wellknown.BackendGVK.Kind, nil, nil)
	}
	maps.Copy(resolvers, aliasTargetResolvers(backendPlugins))
	maps.Copy(resolvers, extra)
	return resolvers
}

// NewObjectPolicyTargetResolver returns a resolver that reports a target missing when col has
// no object under namespace/name. When the ref carries a sectionName and sectionNames is set,
// the section must be one of the names it returns for the object; a nil sectionNames leaves
// sectionName unchecked. kind names the target in the status message, which matches the one
// the built-in resolvers produce. It is the building block for WithPolicyTargetResolver.
func NewObjectPolicyTargetResolver[T controllers.Object](
	col krt.Collection[T],
	kind string,
	sectionNames func(obj T) []string,
) PolicyTargetResolver {
	return objectTargetResolver(col, kind, nil, sectionNames)
}

// objectTargetResolver checks that the named object exists in col and, when set, that matches
// accepts it. When the ref carries a sectionName and sectionNames is set, the section must be
// one of the object's; a nil sectionNames accepts any sectionName unchecked.
func objectTargetResolver[T controllers.Object](
	col krt.Collection[T],
	kind string,
	matches func(obj T) bool,
	sectionNames func(obj T) []string,
) PolicyTargetResolver {
	return func(kctx krt.HandlerContext, namespace, name, sectionName string) error {
		obj := krt.FetchOne(kctx, col, krt.FilterKey(namespace+"/"+name))
		if obj == nil || (matches != nil && !matches(*obj)) {
			return targetNotFoundError(kind, namespace, name)
		}
		if sectionName != "" && sectionNames != nil && !slices.Contains(sectionNames(*obj), sectionName) {
			return fmt.Errorf("sectionName %q not found in %s %s/%s", sectionName, kind, namespace, name)
		}
		return nil
	}
}

// namesOf projects each item to its name. An unnamed item projects to "", which no sectionName
// can equal since refs without one are not section checked.
func namesOf[T any](items []T, name func(T) string) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, name(item))
	}
	return names
}

// aliasTargetResolvers builds a resolver for every alias kind the backend plugins declare. A
// policy targets an alias kind (for example networking.istio.io/Hostname) by name, and the
// backend index attaches it to every backend carrying a matching alias, so the target exists
// when at least one backend of a plugin claiming that kind carries the alias. The lookup mirrors
// the attachment rule: an alias without a namespace is scoped to its backend's namespace.
func aliasTargetResolvers(backendPlugins map[schema.GroupKind]sdk.BackendPlugin) policyTargetResolvers {
	type aliasIndex struct {
		backends krt.Collection[ir.BackendObjectIR]
		byAlias  krt.Index[ir.ObjectSource, ir.BackendObjectIR]
	}
	indexesByAliasKind := map[schema.GroupKind][]aliasIndex{}
	for _, plugin := range backendPlugins {
		if plugin.Backends == nil || len(plugin.AliasKinds) == 0 {
			continue
		}
		idx := aliasIndex{
			backends: plugin.Backends,
			byAlias: utilkrt.UnnamedIndex(plugin.Backends, func(backend ir.BackendObjectIR) []ir.ObjectSource {
				keys := make([]ir.ObjectSource, 0, len(backend.Aliases))
				for _, alias := range backend.Aliases {
					if alias.Namespace == "" {
						alias.Namespace = backend.GetNamespace()
					}
					keys = append(keys, alias)
				}
				return keys
			}),
		}
		for _, gk := range plugin.AliasKinds {
			indexesByAliasKind[gk] = append(indexesByAliasKind[gk], idx)
		}
	}

	resolvers := policyTargetResolvers{}
	for gk, indexes := range indexesByAliasKind {
		resolvers[gk] = func(kctx krt.HandlerContext, namespace, name, _ string) error {
			key := ir.ObjectSource{Group: gk.Group, Kind: gk.Kind, Namespace: namespace, Name: name}
			for _, idx := range indexes {
				if len(krt.Fetch(kctx, idx.backends, krt.FilterIndex(idx.byAlias, key))) > 0 {
					return nil
				}
			}
			return targetNotFoundError(gk.Kind, namespace, name)
		}
	}
	return resolvers
}

func targetNotFoundError(kind, namespace, name string) error {
	return fmt.Errorf("%s %s/%s not found", kind, namespace, name)
}

// policyTargetStatusContributions emits one contribution per policy that has at least one
// explicit targetRef the resolvers cannot resolve, and nothing for every other policy. policies
// may hold every policy kind at once: a PolicyWrapper's key already includes its group and kind.
func policyTargetStatusContributions(
	policies krt.Collection[ir.PolicyWrapper],
	resolvers policyTargetResolvers,
	krtopts krtutil.KrtOptions,
) krt.Collection[reports.StatusContribution] {
	return krt.NewCollection(policies, func(kctx krt.HandlerContext, policy ir.PolicyWrapper) *reports.StatusContribution {
		problems := unresolvedPolicyTargets(kctx, policy, resolvers)
		if len(problems) == 0 {
			return nil
		}
		contribution := policyTargetStatusContribution(policy, problems)
		return &contribution
	}, krtopts.ToOptions("PolicyTargetStatusContributions")...)
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

// policyTargetStatusContribution wraps the policy's target report as the single contribution
// of the policy-target source for that policy.
func policyTargetStatusContribution(policy ir.PolicyWrapper, problems []string) reports.StatusContribution {
	return reports.StatusContribution{
		Target: reports.StatusKey{
			GroupKind:      schema.GroupKind{Group: policy.Group, Kind: policy.Kind},
			NamespacedName: types.NamespacedName{Namespace: policy.Namespace, Name: policy.Name},
		},
		Source: reports.StatusSource{
			Kind: reports.PolicyTargetStatusSource,
			Name: policy.ResourceName(),
		},
		StatusReport: reports.StatusReport{Policy: buildPolicyTargetReport(policy, problems)},
	}
}

// buildPolicyTargetReport reports the unresolved targets on the policy's summary ancestor.
func buildPolicyTargetReport(policy ir.PolicyWrapper, problems []string) *reports.PolicyReport {
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
	ancestor := reports.NewReporter(&reportMap).Policy(key, generation).AncestorRef(reporter.PolicyStatusSummaryAncestorRef())
	// The standard policy builder stamps the report's generation onto every condition, and the
	// reducer keeps the generation of whichever contribution sorts first for the policy, so the
	// per-condition value below only matters for builders that copy conditions verbatim, such
	// as BackendTLSPolicy's.
	ancestor.SetCondition(reporter.PolicyCondition{
		Type:               string(shared.PolicyConditionAccepted),
		Status:             metav1.ConditionFalse,
		Reason:             string(shared.PolicyReasonTargetNotFound),
		Message:            strings.Join(problems, "; "),
		ObservedGeneration: generation,
	})
	ancestor.SetCondition(reporter.PolicyCondition{
		Type:               string(shared.PolicyConditionAttached),
		Status:             metav1.ConditionFalse,
		Reason:             string(shared.PolicyReasonTargetNotFound),
		Message:            reporter.PolicyTargetNotFoundMsg,
		ObservedGeneration: generation,
	})
	return reportMap.PolicyReport(key)
}

// GeneratePolicyTargetReports resolves every policy's explicit targetRefs once, outside a krt
// transform, and returns the TargetNotFound reports keyed by policy: the same reports
// policyTargetStatusContributions produces incrementally in the proxy syncer. Exported for
// the translator golden tests, which build status from report maps rather than running the
// syncer. opts are the options the syncer would be given: resolvers registered through
// WithPolicyTargetResolver are applied as they are there, and every other setting is ignored.
// Resolvers run here with a dummy krt context, so the collections they read must already be
// synced.
func GeneratePolicyTargetReports(
	commonCols *collections.CommonCollections,
	plugins sdk.Plugin,
	opts ...StatusSyncerOption,
) reports.ReportMap {
	extra := processStatusSyncerOptions(opts...).policyTargetResolvers
	resolvers := newPolicyTargetResolvers(commonCols, plugins.ContributesBackends, extra)
	out := reports.NewPolicyReportMap()
	for _, plugin := range plugins.ContributesPolicies {
		if plugin.Policies == nil {
			continue
		}
		for _, policy := range plugin.Policies.List() {
			problems := unresolvedPolicyTargets(krt.TestingDummyContext{}, policy, resolvers)
			if len(problems) == 0 {
				continue
			}
			key := reporter.PolicyKey{Group: policy.Group, Kind: policy.Kind, Namespace: policy.Namespace, Name: policy.Name}
			out.Policies[key] = buildPolicyTargetReport(policy, problems)
		}
	}
	return out
}
