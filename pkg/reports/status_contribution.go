package reports

import (
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

// StatusKey is the version-independent identity of a status owner. Multiple
// served API versions of a Gateway API resource share storage and status, and
// the version a status is written back through comes from the object's own
// TypeMeta at write time, never from a contribution.
type StatusKey struct {
	schema.GroupKind
	types.NamespacedName
}

func (k StatusKey) String() string {
	return k.Group + "/" + k.Kind + "/" + k.Namespace + "/" + k.Name
}

type StatusSourceKind string

const (
	GatewayStatusSource       StatusSourceKind = "gateway"
	BackendPolicyStatusSource StatusSourceKind = "backend-policy"
	BackendStatusSource       StatusSourceKind = "backend-status"
	// PolicyTargetStatusSource contributions come from checking a policy's own targetRefs
	// against the objects they name, independently of any Gateway translation. They are the
	// only source that can report on a policy whose target does not exist.
	PolicyTargetStatusSource StatusSourceKind = "policy-target"
	// XdsRejectionStatusSource contributions report that connected proxies rejected a
	// Gateway's xDS configuration. Unlike other Gateway contributions they are not complete
	// reports: the reducer overlays their conditions on the Gateway's translation report, and
	// drops them when there is no translation report to overlay.
	XdsRejectionStatusSource StatusSourceKind = "xds-rejection"
)

// StatusSource identifies the translation unit that produced a contribution.
// Kind is used for semantic selection; Name provides uniqueness within that
// producer kind.
type StatusSource struct {
	Kind StatusSourceKind
	Name string
}

func (s StatusSource) String() string {
	return string(s.Kind) + "/" + s.Name
}

// StatusReport is the compact report fragment retained for one status owner.
// Exactly one field is populated.
type StatusReport struct {
	Gateway     *GatewayReport
	ListenerSet *ListenerSetReport
	Route       *RouteReport
	Policy      *PolicyReport
	Backend     *BackendReport
}

func (r StatusReport) Equals(other StatusReport) bool {
	return gatewayReportEqual(r.Gateway, other.Gateway) &&
		listenerSetReportEqual(r.ListenerSet, other.ListenerSet) &&
		routeReportEqual(r.Route, other.Route) &&
		policyReportEqual(r.Policy, other.Policy) &&
		backendReportEqual(r.Backend, other.Backend)
}

// StatusContribution is one translation unit's status facts for one Kubernetes
// object. Exactly one report field is populated. Source is a stable identity for
// the producer (for example, a Gateway or Backend), allowing multiple producers
// to contribute independently to the same target.
type StatusContribution struct {
	Target StatusKey
	Source StatusSource
	StatusReport
}

func (c StatusContribution) ResourceName() string {
	return c.Target.Group + "/" + c.Target.Kind + "/" + c.Target.Namespace + "/" + c.Target.Name + "/" +
		string(c.Source.Kind) + "/" + c.Source.Name
}

func (c StatusContribution) Equals(other StatusContribution) bool {
	return c.Target == other.Target &&
		c.Source == other.Source &&
		c.StatusReport.Equals(other.StatusReport)
}

// StatusContributionsFromReportMap splits a translation-local ReportMap into
// independently keyed status contributions. It transfers ownership of the
// report fragments to the returned contributions; callers must not mutate the
// input afterward. The result is deterministically sorted without formatting
// keys in the comparator.
func StatusContributionsFromReportMap(source StatusSource, reportMap ReportMap) []StatusContribution {
	listenerSetCount := 0
	for _, byName := range reportMap.ListenerSets {
		listenerSetCount += len(byName)
	}
	contributions := make([]StatusContribution, 0,
		len(reportMap.Gateways)+listenerSetCount+
			len(reportMap.HTTPRoutes)+len(reportMap.GRPCRoutes)+
			len(reportMap.TCPRoutes)+len(reportMap.TLSRoutes)+
			len(reportMap.Policies)+len(reportMap.Backends),
	)

	for nn, report := range reportMap.Gateways {
		if report != nil {
			contributions = append(contributions, StatusContribution{
				Target:       StatusKey{GroupKind: wellknown.GatewayGVK.GroupKind(), NamespacedName: nn},
				Source:       source,
				StatusReport: StatusReport{Gateway: report},
			})
		}
	}
	for gvk, byName := range reportMap.ListenerSets {
		for nn, report := range byName {
			if report != nil {
				contributions = append(contributions, StatusContribution{
					Target:       StatusKey{GroupKind: gvk.GroupKind(), NamespacedName: nn},
					Source:       source,
					StatusReport: StatusReport{ListenerSet: report},
				})
			}
		}
	}
	appendRoutes := func(gvk schema.GroupVersionKind, reportsByName map[types.NamespacedName]*RouteReport) {
		for nn, report := range reportsByName {
			if report != nil {
				contributions = append(contributions, StatusContribution{
					Target:       StatusKey{GroupKind: gvk.GroupKind(), NamespacedName: nn},
					Source:       source,
					StatusReport: StatusReport{Route: report},
				})
			}
		}
	}
	appendRoutes(wellknown.HTTPRouteGVK, reportMap.HTTPRoutes)
	appendRoutes(wellknown.GRPCRouteGVK, reportMap.GRPCRoutes)
	appendRoutes(wellknown.TCPRouteGVK, reportMap.TCPRoutes)
	appendRoutes(wellknown.TLSRouteGVK, reportMap.TLSRoutes)

	for key, report := range reportMap.Policies {
		if report != nil {
			contributions = append(contributions, StatusContribution{
				Target: StatusKey{
					GroupKind:      schema.GroupKind{Group: key.Group, Kind: key.Kind},
					NamespacedName: types.NamespacedName{Namespace: key.Namespace, Name: key.Name},
				},
				Source:       source,
				StatusReport: StatusReport{Policy: report},
			})
		}
	}
	for nn, report := range reportMap.Backends {
		if report != nil {
			contributions = append(contributions, StatusContribution{
				Target:       StatusKey{GroupKind: wellknown.BackendGVK.GroupKind(), NamespacedName: nn},
				Source:       source,
				StatusReport: StatusReport{Backend: report},
			})
		}
	}

	slices.SortFunc(contributions, compareStatusContributions)
	return contributions
}

// compareStatusContributions orders contributions by their complete identity, so no ties
// remain and a stable sort is not needed. The comparisons are chained rather than passed to
// cmp.Or because cmp.Or is an ordinary variadic call: it would run all six on every
// comparison, and this sorts one entry per translated route on every Gateway translation.
func compareStatusContributions(a, b StatusContribution) int {
	if c := strings.Compare(a.Target.Group, b.Target.Group); c != 0 {
		return c
	}
	if c := strings.Compare(a.Target.Kind, b.Target.Kind); c != 0 {
		return c
	}
	if c := strings.Compare(a.Target.Namespace, b.Target.Namespace); c != 0 {
		return c
	}
	if c := strings.Compare(a.Target.Name, b.Target.Name); c != 0 {
		return c
	}
	if c := strings.Compare(string(a.Source.Kind), string(b.Source.Kind)); c != 0 {
		return c
	}
	return strings.Compare(a.Source.Name, b.Source.Name)
}

// ReduceStatusContributions combines contributions for one target into a
// compact fragment. The result owns its reports and can be retained safely.
//
// It reorders contributions in place. The production caller passes a krt.Fetch result,
// which is freshly allocated per call — the index Lookup builds the slice, and Fetch itself
// already FilterInPlaces it — and this runs once per status owner on every recompute that
// touches it, so a defensive copy here is one wasted allocation per owner per event.
// That is a coupling to krt's allocation behavior, so it is pinned by
// TestFetchedContributionsAreNotAliasedByIndexStorage in pkg/pluginsdk/statussync: an upstream
// bump that started handing back index-owned storage fails there rather than silently
// corrupting the index. Callers that need their own ordering back must pass a copy.
func ReduceStatusContributions(contributions []StatusContribution) StatusReport {
	slices.SortFunc(contributions, compareStatusContributions)
	var reduced StatusReport
	// Only the source of the previously seen contribution needs tracking: the reports
	// themselves say whether one was seen, since a non-nil contribution always clones to a
	// non-nil report.
	var gatewaySource, listenerSetSource, backendSource StatusSource
	var gatewayOverlay *GatewayReport
	for _, contribution := range contributions {
		switch {
		case contribution.Gateway != nil && contribution.Source.Kind == XdsRejectionStatusSource:
			gatewayOverlay = contribution.Gateway
		case contribution.Gateway != nil:
			warnOnMultipleSingleWriterContributions("Gateway", contribution, gatewaySource, reduced.Gateway != nil)
			reduced.Gateway = cloneGatewayReport(contribution.Gateway)
			gatewaySource = contribution.Source
		case contribution.ListenerSet != nil:
			warnOnMultipleSingleWriterContributions("ListenerSet", contribution, listenerSetSource, reduced.ListenerSet != nil)
			reduced.ListenerSet = cloneListenerSetReport(contribution.ListenerSet)
			listenerSetSource = contribution.Source
		case contribution.Route != nil:
			if reduced.Route == nil {
				reduced.Route = cloneRouteReport(contribution.Route)
			} else {
				mergeParentReports(reduced.Route, contribution.Route)
			}
		case contribution.Policy != nil:
			if reduced.Policy == nil {
				reduced.Policy = clonePolicyReport(contribution.Policy)
			} else {
				mergeAncestorReports(reduced.Policy, contribution.Policy)
			}
		case contribution.Backend != nil:
			warnOnMultipleSingleWriterContributions("Backend", contribution, backendSource, reduced.Backend != nil)
			reduced.Backend = cloneBackendReport(contribution.Backend)
			backendSource = contribution.Source
		}
	}
	if reduced.Gateway != nil && gatewayOverlay != nil {
		overlayGatewayConditions(reduced.Gateway, gatewayOverlay)
	}
	return reduced
}

// NewXdsRejectionContribution reports that connected proxies of gw rejected its xDS
// configuration, as Programmed=False with reason XdsRejected.
func NewXdsRejectionContribution(gw types.NamespacedName, message string) StatusContribution {
	report := &GatewayReport{}
	report.SetCondition(reporter.GatewayCondition{
		Type:    gwv1.GatewayConditionProgrammed,
		Status:  metav1.ConditionFalse,
		Reason:  reporter.GatewayXdsRejectedReason,
		Message: message,
	})
	return StatusContribution{
		Target:       StatusKey{GroupKind: wellknown.GatewayGVK.GroupKind(), NamespacedName: gw},
		Source:       StatusSource{Kind: XdsRejectionStatusSource, Name: gw.String()},
		StatusReport: StatusReport{Gateway: report},
	}
}

// overlayGatewayConditions sets overlay's conditions on report, except where report already
// has a False condition of the same type: translation's reason is the more fundamental one,
// and a proxy rejecting config that translation already marked invalid adds nothing.
func overlayGatewayConditions(report, overlay *GatewayReport) {
	for _, condition := range overlay.conditions {
		if existing := meta.FindStatusCondition(report.conditions, condition.Type); existing != nil &&
			existing.Status == metav1.ConditionFalse {
			continue
		}
		meta.SetStatusCondition(&report.conditions, condition)
	}
}

// Gateway, ListenerSet, and Backend reports are complete owner snapshots, not
// independently keyed facts like route parents or policy ancestors. Their
// current producers are therefore intentionally single-writer. Keep the
// deterministic last-writer behavior for resilience, but make any violation of
// that topology visible rather than silently discarding one producer's report.
func warnOnMultipleSingleWriterContributions(kind string, replacement StatusContribution, previous StatusSource, hasPrevious bool) {
	if !hasPrevious {
		return
	}
	logger.Warn("multiple status contributions for single-writer report kind; replacing earlier contribution",
		"report_kind", kind,
		"target", replacement.Target.String(),
		"previous_source", previous.String(),
		"replacement_source", replacement.Source.String(),
	)
}
