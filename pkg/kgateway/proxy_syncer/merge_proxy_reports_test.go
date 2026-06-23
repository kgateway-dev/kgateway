package proxy_syncer

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// routeReportWithParent builds a per-proxy ReportMap containing a single HTTPRoute
// report whose only parent is the Gateway named gwName.
func routeReportWithParent(routeName, gwName string) reports.ReportMap {
	rm := reports.NewReportMap()
	r := reports.NewReporter(&rm)
	route := &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: routeName, Generation: 1}}
	parent := gwv1.ParentReference{
		Group:     new(gwv1.Group("gateway.networking.k8s.io")),
		Kind:      new(gwv1.Kind("Gateway")),
		Name:      gwv1.ObjectName(gwName),
		Namespace: new(gwv1.Namespace("default")),
	}
	r.Route(route).ParentRef(&parent).SetCondition(reporter.RouteCondition{
		Type:    gwv1.RouteConditionAccepted,
		Status:  metav1.ConditionTrue,
		Reason:  gwv1.RouteReasonAccepted,
		Message: "accepted",
	})
	return rm
}

// TestMergeProxyReportsDoesNotMutateProxyReports ensures that merging two proxies
// that report the same route under different parents does not mutate either
// per-proxy report. Those reports are owned by the mostXdsSnapshots collection and
// are read concurrently for equality checks, so mergeProxyReports must produce
// owned copies before merging parents.
func TestMergeProxyReportsDoesNotMutateProxyReports(t *testing.T) {
	routeKey := types.NamespacedName{Namespace: "default", Name: "route"}

	proxyA := GatewayXdsResources{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "gw-a"},
		reports:        routeReportWithParent("route", "gw-a"),
	}
	proxyB := GatewayXdsResources{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "gw-b"},
		reports:        routeReportWithParent("route", "gw-b"),
	}

	// Snapshot the original parent pointers and counts for proxyA's report.
	aReport := proxyA.reports.HTTPRoutes[routeKey]
	if got := len(aReport.Parents); got != 1 {
		t.Fatalf("precondition: expected proxyA route report to have 1 parent, got %d", got)
	}

	merged := mergeProxyReports([]GatewayXdsResources{proxyA, proxyB})

	// The merged report must contain both parents.
	mergedRoute := merged.HTTPRoutes[routeKey]
	if mergedRoute == nil {
		t.Fatal("expected merged report to contain the route")
	}
	if got := len(mergedRoute.Parents); got != 2 {
		t.Fatalf("expected merged route report to have 2 parents, got %d", got)
	}

	// The per-proxy reports must be untouched.
	if got := len(proxyA.reports.HTTPRoutes[routeKey].Parents); got != 1 {
		t.Fatalf("proxyA report was mutated: expected 1 parent, got %d", got)
	}
	if got := len(proxyB.reports.HTTPRoutes[routeKey].Parents); got != 1 {
		t.Fatalf("proxyB report was mutated: expected 1 parent, got %d", got)
	}

	// The merged route report must be a distinct object from the per-proxy reports.
	if mergedRoute == proxyA.reports.HTTPRoutes[routeKey] {
		t.Fatal("merged route report aliases proxyA's per-proxy report")
	}
}
