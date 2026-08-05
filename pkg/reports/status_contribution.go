package reports

import (
	"cmp"
	"log/slog"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// StatusTarget identifies the Kubernetes object that owns a status. Version is
// retained for resources whose promoted and legacy forms have different status
// writers; callers that normalize versions may use the selected write version.
type StatusTarget struct {
	schema.GroupVersionKind
	types.NamespacedName
}

func (t StatusTarget) String() string {
	return t.Group + "/" + t.Version + "/" + t.Kind + "/" + t.Namespace + "/" + t.Name
}

// StatusKey is the version-independent identity of a status owner. Multiple
// served API versions of a Gateway API resource share storage and status.
type StatusKey struct {
	schema.GroupKind
	types.NamespacedName
}

func (k StatusKey) String() string {
	return k.Group + "/" + k.Kind + "/" + k.Namespace + "/" + k.Name
}

func (t StatusTarget) Key() StatusKey {
	return StatusKey{GroupKind: t.GroupKind(), NamespacedName: t.NamespacedName}
}

type StatusSourceKind string

const (
	GatewayStatusSource       StatusSourceKind = "gateway"
	BackendPolicyStatusSource StatusSourceKind = "backend-policy"
	BackendStatusSource       StatusSourceKind = "backend-status"
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
	Target StatusTarget
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
				Target:       StatusTarget{GroupVersionKind: wellknown.GatewayGVK, NamespacedName: nn},
				Source:       source,
				StatusReport: StatusReport{Gateway: report},
			})
		}
	}
	for gvk, byName := range reportMap.ListenerSets {
		for nn, report := range byName {
			if report != nil {
				contributions = append(contributions, StatusContribution{
					Target:       StatusTarget{GroupVersionKind: gvk, NamespacedName: nn},
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
					Target:       StatusTarget{GroupVersionKind: gvk, NamespacedName: nn},
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
				Target: StatusTarget{
					GroupVersionKind: schema.GroupVersionKind{Group: key.Group, Kind: key.Kind},
					NamespacedName:   types.NamespacedName{Namespace: key.Namespace, Name: key.Name},
				},
				Source:       source,
				StatusReport: StatusReport{Policy: report},
			})
		}
	}
	for nn, report := range reportMap.Backends {
		if report != nil {
			contributions = append(contributions, StatusContribution{
				Target:       StatusTarget{GroupVersionKind: wellknown.BackendGVK, NamespacedName: nn},
				Source:       source,
				StatusReport: StatusReport{Backend: report},
			})
		}
	}

	slices.SortStableFunc(contributions, compareStatusContributions)
	return contributions
}

func compareStatusContributions(a, b StatusContribution) int {
	return cmp.Or(
		strings.Compare(a.Target.Group, b.Target.Group),
		strings.Compare(a.Target.Kind, b.Target.Kind),
		strings.Compare(a.Target.Namespace, b.Target.Namespace),
		strings.Compare(a.Target.Name, b.Target.Name),
		strings.Compare(string(a.Source.Kind), string(b.Source.Kind)),
		strings.Compare(a.Source.Name, b.Source.Name),
		strings.Compare(a.Target.Version, b.Target.Version),
	)
}

// ReduceStatusContributions combines contributions for one target into a
// compact fragment. The result owns its reports and can be retained safely.
func ReduceStatusContributions(contributions []StatusContribution) StatusReport {
	ordered := slices.Clone(contributions)
	slices.SortStableFunc(ordered, compareStatusContributions)
	var reduced StatusReport
	var gatewaySource, listenerSetSource, backendSource StatusSource
	var hasGateway, hasListenerSet, hasBackend bool
	for _, contribution := range ordered {
		switch {
		case contribution.Gateway != nil:
			warnOnMultipleSingleWriterContributions("Gateway", contribution, gatewaySource, hasGateway)
			reduced.Gateway = cloneGatewayReport(contribution.Gateway)
			gatewaySource, hasGateway = contribution.Source, true
		case contribution.ListenerSet != nil:
			warnOnMultipleSingleWriterContributions("ListenerSet", contribution, listenerSetSource, hasListenerSet)
			reduced.ListenerSet = cloneListenerSetReport(contribution.ListenerSet)
			listenerSetSource, hasListenerSet = contribution.Source, true
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
			warnOnMultipleSingleWriterContributions("Backend", contribution, backendSource, hasBackend)
			reduced.Backend = cloneBackendReport(contribution.Backend)
			backendSource, hasBackend = contribution.Source, true
		}
	}
	return reduced
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
	slog.Warn("multiple status contributions for single-writer report kind; replacing earlier contribution",
		"report_kind", kind,
		"target", replacement.Target.Key().String(),
		"previous_source", previous.String(),
		"replacement_source", replacement.Source.String(),
	)
}
