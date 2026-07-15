package reports

import (
	"maps"
	"slices"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// MergeReportMaps returns a report map owned by the caller. The returned reports
// do not alias the input report objects, so later status rendering and status
// marker processing can safely mutate the merged map without writing into
// per-translation reports.
func MergeReportMaps(inputs ...ReportMap) ReportMap {
	merged := NewReportMap()
	for _, input := range inputs {
		for key, report := range input.Gateways {
			merged.Gateways[key] = cloneGatewayReport(report)
		}
		for gvk, reportsByName := range input.ListenerSets {
			if merged.ListenerSets[gvk] == nil {
				merged.ListenerSets[gvk] = make(map[types.NamespacedName]*ListenerSetReport, len(reportsByName))
			}
			for key, report := range reportsByName {
				merged.ListenerSets[gvk][key] = cloneListenerSetReport(report)
			}
		}
		mergeRouteReportMap(merged.HTTPRoutes, input.HTTPRoutes)
		mergeRouteReportMap(merged.GRPCRoutes, input.GRPCRoutes)
		mergeRouteReportMap(merged.TCPRoutes, input.TCPRoutes)
		mergeRouteReportMap(merged.TLSRoutes, input.TLSRoutes)
		for key, report := range input.Policies {
			existing := merged.Policies[key]
			if existing == nil {
				merged.Policies[key] = clonePolicyReport(report)
				continue
			}
			mergeAncestorReports(existing, report)
		}
	}
	return merged
}

func mergeRouteReportMap(dst, src map[types.NamespacedName]*RouteReport) {
	for key, report := range src {
		existing := dst[key]
		if existing == nil {
			dst[key] = cloneRouteReport(report)
			continue
		}
		mergeParentReports(existing, report)
	}
}

func mergeParentReports(dst, src *RouteReport) {
	if dst == nil || src == nil {
		return
	}
	if len(src.Parents) == 0 {
		return
	}
	if dst.Parents == nil {
		dst.Parents = make(map[ParentRefKey]*ParentRefReport, len(src.Parents))
	}
	for key, report := range src.Parents {
		dst.Parents[key] = cloneParentRefReport(report)
	}
}

func mergeAncestorReports(dst, src *PolicyReport) {
	if dst == nil || src == nil {
		return
	}
	if len(src.Ancestors) == 0 {
		return
	}
	if dst.Ancestors == nil {
		dst.Ancestors = make(map[ParentRefKey]*AncestorRefReport, len(src.Ancestors))
	}
	for key, report := range src.Ancestors {
		dst.Ancestors[key] = cloneAncestorRefReport(report)
	}
}

func cloneGatewayReport(in *GatewayReport) *GatewayReport {
	if in == nil {
		return nil
	}
	return &GatewayReport{
		conditions:           slices.Clone(in.conditions),
		listeners:            cloneListenerReports(in.listeners),
		observedGeneration:   in.observedGeneration,
		attachedListenerSets: in.attachedListenerSets,
	}
}

func cloneListenerSetReport(in *ListenerSetReport) *ListenerSetReport {
	if in == nil {
		return nil
	}
	return &ListenerSetReport{
		conditions:         slices.Clone(in.conditions),
		listeners:          cloneListenerReports(in.listeners),
		observedGeneration: in.observedGeneration,
	}
}

func cloneListenerReports(in map[string]*ListenerReport) map[string]*ListenerReport {
	if in == nil {
		return nil
	}
	out := make(map[string]*ListenerReport, len(in))
	for key, report := range in {
		if report == nil {
			out[key] = nil
			continue
		}
		status := report.Status
		status.Conditions = slices.Clone(status.Conditions)
		status.SupportedKinds = slices.Clone(status.SupportedKinds)
		out[key] = &ListenerReport{Status: status}
	}
	return out
}

func cloneRouteReport(in *RouteReport) *RouteReport {
	if in == nil {
		return nil
	}
	out := &RouteReport{observedGeneration: in.observedGeneration}
	if in.Parents != nil {
		out.Parents = make(map[ParentRefKey]*ParentRefReport, len(in.Parents))
		for key, report := range in.Parents {
			out.Parents[key] = cloneParentRefReport(report)
		}
	}
	return out
}

func cloneParentRefReport(in *ParentRefReport) *ParentRefReport {
	if in == nil {
		return nil
	}
	return &ParentRefReport{Conditions: slices.Clone(in.Conditions)}
}

func clonePolicyReport(in *PolicyReport) *PolicyReport {
	if in == nil {
		return nil
	}
	out := &PolicyReport{observedGeneration: in.observedGeneration}
	if in.Ancestors != nil {
		out.Ancestors = make(map[ParentRefKey]*AncestorRefReport, len(in.Ancestors))
		for key, report := range in.Ancestors {
			out.Ancestors[key] = cloneAncestorRefReport(report)
		}
	}
	return out
}

func cloneAncestorRefReport(in *AncestorRefReport) *AncestorRefReport {
	if in == nil {
		return nil
	}
	return &AncestorRefReport{
		Conditions:      slices.Clone(in.Conditions),
		AttachmentState: in.AttachmentState,
	}
}

// EqualReportMaps compares report maps by semantic report contents rather than
// report pointer identity. LastTransitionTime is intentionally ignored so
// timestamp-only status changes do not cause KRT churn.
func EqualReportMaps(a, b ReportMap) bool {
	return maps.EqualFunc(a.Gateways, b.Gateways, gatewayReportEqual) &&
		listenerSetMapsEqual(a.ListenerSets, b.ListenerSets) &&
		maps.EqualFunc(a.HTTPRoutes, b.HTTPRoutes, routeReportEqual) &&
		maps.EqualFunc(a.GRPCRoutes, b.GRPCRoutes, routeReportEqual) &&
		maps.EqualFunc(a.TCPRoutes, b.TCPRoutes, routeReportEqual) &&
		maps.EqualFunc(a.TLSRoutes, b.TLSRoutes, routeReportEqual) &&
		maps.EqualFunc(a.Policies, b.Policies, policyReportEqual)
}

func listenerSetMapsEqual(
	a, b map[schema.GroupVersionKind]map[types.NamespacedName]*ListenerSetReport,
) bool {
	return maps.EqualFunc(a, b, func(a, b map[types.NamespacedName]*ListenerSetReport) bool {
		return maps.EqualFunc(a, b, listenerSetReportEqual)
	})
}

func gatewayReportEqual(a, b *GatewayReport) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.observedGeneration == b.observedGeneration &&
		a.attachedListenerSets == b.attachedListenerSets &&
		conditionsEqual(a.conditions, b.conditions) &&
		maps.EqualFunc(a.listeners, b.listeners, listenerReportEqual)
}

func listenerSetReportEqual(a, b *ListenerSetReport) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.observedGeneration == b.observedGeneration &&
		conditionsEqual(a.conditions, b.conditions) &&
		maps.EqualFunc(a.listeners, b.listeners, listenerReportEqual)
}

func listenerReportEqual(a, b *ListenerReport) bool {
	if a == nil || b == nil {
		return a == b
	}
	return listenerStatusEqual(a.Status, b.Status)
}

func listenerStatusEqual(a, b gwv1.ListenerStatus) bool {
	return a.Name == b.Name &&
		a.AttachedRoutes == b.AttachedRoutes &&
		slices.EqualFunc(a.SupportedKinds, b.SupportedKinds, routeGroupKindEqual) &&
		conditionsEqual(a.Conditions, b.Conditions)
}

func routeGroupKindEqual(a, b gwv1.RouteGroupKind) bool {
	return ptrValueEqual(a.Group, b.Group) && a.Kind == b.Kind
}

func ptrValueEqual[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func routeReportEqual(a, b *RouteReport) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.observedGeneration == b.observedGeneration &&
		maps.EqualFunc(a.Parents, b.Parents, parentRefReportEqual)
}

func parentRefReportEqual(a, b *ParentRefReport) bool {
	if a == nil || b == nil {
		return a == b
	}
	return conditionsEqual(a.Conditions, b.Conditions)
}

func policyReportEqual(a, b *PolicyReport) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.observedGeneration == b.observedGeneration &&
		maps.EqualFunc(a.Ancestors, b.Ancestors, ancestorRefReportEqual)
}

func ancestorRefReportEqual(a, b *AncestorRefReport) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.AttachmentState == b.AttachmentState &&
		conditionsEqual(a.Conditions, b.Conditions)
}

func conditionsEqual(a, b []metav1.Condition) bool {
	if len(a) != len(b) {
		return false
	}
	for _, condition := range a {
		other := meta.FindStatusCondition(b, condition.Type)
		if other == nil ||
			condition.Status != other.Status ||
			condition.Reason != other.Reason ||
			condition.Message != other.Message ||
			condition.ObservedGeneration != other.ObservedGeneration {
			return false
		}
	}
	return true
}
