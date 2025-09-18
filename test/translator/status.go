package translator

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwxv1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

type Statuses struct {
	Gateways     map[string]*gwv1.GatewayStatus      `json:"gateways,omitempty"`
	ListenerSets map[string]*gwxv1.ListenerSetStatus `json:"listenerSets,omitempty"`
	HTTPRoutes   map[string]*gwv1.RouteStatus        `json:"httpRoutes,omitempty"`
	TCPRoutes    map[string]*gwv1.RouteStatus        `json:"tcpRoutes,omitempty"`
	TLSRoutes    map[string]*gwv1.RouteStatus        `json:"tlsRoutes,omitempty"`
	GRPCRoutes   map[string]*gwv1.RouteStatus        `json:"grpcRoutes,omitempty"`
	Policies     map[string]*gwv1a2.PolicyStatus     `json:"policies,omitempty"`
}

func buildStatusesFromReports(
	reportsMap reports.ReportMap,
	gateways map[types.NamespacedName]*gwv1.Gateway,
	listenerSets map[types.NamespacedName]*gwxv1.XListenerSet,
) *Statuses {
	ctx := context.Background()

	// Fixed values for deterministic golden file tests. Use the zero time
	// for consistency and to avoid confusion about the significance of a
	// specific date.
	fixedTime := metav1.Time{Time: time.Time{}}

	statuses := &Statuses{
		Gateways:     make(map[string]*gwv1.GatewayStatus),
		ListenerSets: make(map[string]*gwxv1.ListenerSetStatus),
		HTTPRoutes:   make(map[string]*gwv1.RouteStatus),
		TCPRoutes:    make(map[string]*gwv1.RouteStatus),
		TLSRoutes:    make(map[string]*gwv1.RouteStatus),
		GRPCRoutes:   make(map[string]*gwv1.RouteStatus),
		Policies:     make(map[string]*gwv1a2.PolicyStatus),
	}

	// Build Gateway statuses. We need to use the actual Gateway object to make sure that
	// status.listeners are correctly populated instead of using the object metadata.
	for gwNN := range reportsMap.Gateways {
		// Use the actual Gateway object from the input if available, otherwise create empty one
		var gw gwv1.Gateway
		if actualGw, exists := gateways[gwNN]; exists && actualGw != nil {
			gw = *actualGw
		} else {
			gw = gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gwNN.Name,
					Namespace: gwNN.Namespace,
				},
			}
		}
		if status := reportsMap.BuildGWStatus(ctx, gw, nil); status != nil {
			normalizeStatus(status, fixedTime)
			statuses.Gateways[gwNN.String()] = status
		}
	}

	// Build ListenerSet statuses. We need to use the actual XListenerSet object to make sure that
	// status.listeners are correctly populated instead of using the object metadata.
	for listenerSetNN := range reportsMap.ListenerSets {
		// Use the actual XListenerSet object from the input if available, otherwise create empty one
		var listenerSet gwxv1.XListenerSet
		if actualLS, exists := listenerSets[listenerSetNN]; exists && actualLS != nil {
			listenerSet = *actualLS
		} else {
			listenerSet = gwxv1.XListenerSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      listenerSetNN.Name,
					Namespace: listenerSetNN.Namespace,
				},
			}
		}
		if status := reportsMap.BuildListenerSetStatus(ctx, listenerSet); status != nil {
			normalizeListenerSetStatus(status, fixedTime)
			statuses.ListenerSets[listenerSetNN.String()] = status
		}
	}

	// Build HTTPRoute statuses
	for routeNN := range reportsMap.HTTPRoutes {
		route := gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeNN.Name,
				Namespace: routeNN.Namespace,
			},
		}
		if status := reportsMap.BuildRouteStatus(ctx, &route, wellknown.DefaultGatewayClassName); status != nil {
			normalizeRouteStatus(status, fixedTime)
			statuses.HTTPRoutes[routeNN.String()] = status
		}
	}

	// Build TCPRoute statuses
	for routeNN := range reportsMap.TCPRoutes {
		route := gwv1a2.TCPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeNN.Name,
				Namespace: routeNN.Namespace,
			},
		}
		if status := reportsMap.BuildRouteStatus(ctx, &route, wellknown.DefaultGatewayClassName); status != nil {
			normalizeRouteStatus(status, fixedTime)
			statuses.TCPRoutes[routeNN.String()] = status
		}
	}

	// Build TLSRoute statuses
	for routeNN := range reportsMap.TLSRoutes {
		route := gwv1a2.TLSRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeNN.Name,
				Namespace: routeNN.Namespace,
			},
		}
		if status := reportsMap.BuildRouteStatus(ctx, &route, wellknown.DefaultGatewayClassName); status != nil {
			normalizeRouteStatus(status, fixedTime)
			statuses.TLSRoutes[routeNN.String()] = status
		}
	}

	// Build GRPCRoute statuses
	for routeNN := range reportsMap.GRPCRoutes {
		route := gwv1.GRPCRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeNN.Name,
				Namespace: routeNN.Namespace,
			},
		}
		if status := reportsMap.BuildRouteStatus(ctx, &route, wellknown.DefaultGatewayClassName); status != nil {
			normalizeRouteStatus(status, fixedTime)
			statuses.GRPCRoutes[routeNN.String()] = status
		}
	}

	// Build Policy statuses
	for policyKey := range reportsMap.Policies {
		policyKeyStr := fmt.Sprintf("%s/%s/%s", policyKey.Kind, policyKey.Namespace, policyKey.Name)
		if status := reportsMap.BuildPolicyStatus(ctx, policyKey, wellknown.DefaultGatewayControllerName, gwv1a2.PolicyStatus{}); status != nil {
			normalizePolicyStatus(status, fixedTime)
			statuses.Policies[policyKeyStr] = status
		}
	}

	return statuses
}

// normalizeStatus sets all fields (e.g. LastTransitionTime) to fixed values for deterministic testing
func normalizeStatus(status *gwv1.GatewayStatus, time metav1.Time) {
	for i := range status.Conditions {
		status.Conditions[i].LastTransitionTime = time
		for _, listener := range status.Listeners {
			for j := range listener.Conditions {
				listener.Conditions[j].LastTransitionTime = time
			}
		}
	}
}

// normalizeListenerSetStatus sets all fields (e.g. LastTransitionTime) to fixed values for deterministic testing
func normalizeListenerSetStatus(status *gwxv1.ListenerSetStatus, time metav1.Time) {
	for i := range status.Conditions {
		status.Conditions[i].LastTransitionTime = time
		for _, listener := range status.Listeners {
			for j := range listener.Conditions {
				listener.Conditions[j].LastTransitionTime = time
			}
		}
	}
}

// normalizeRouteStatus sets all fields (e.g. LastTransitionTime) to fixed values for deterministic testing
func normalizeRouteStatus(status *gwv1.RouteStatus, time metav1.Time) {
	for i := range status.Parents {
		for j := range status.Parents[i].Conditions {
			status.Parents[i].Conditions[j].LastTransitionTime = time
		}
	}
}

// normalizePolicyStatus sets all fields (e.g. LastTransitionTime) to fixed values for deterministic testing
func normalizePolicyStatus(status *gwv1a2.PolicyStatus, time metav1.Time) {
	for i := range status.Ancestors {
		for j := range status.Ancestors[i].Conditions {
			status.Ancestors[i].Conditions[j].LastTransitionTime = time
		}
	}
}

func compareStatuses(expectedFile string, actualStatuses *Statuses) (string, error) {
	expectedOutput := &translationResult{}
	if err := ReadYamlFile(expectedFile, expectedOutput); err != nil {
		return "", err
	}

	if expectedOutput.Statuses == nil && actualStatuses == nil {
		return "", nil
	}
	if expectedOutput.Statuses == nil {
		return "expected no statuses but got some", nil
	}
	if actualStatuses == nil {
		return "expected statuses but got none", nil
	}

	// Sort statuses for consistent comparison
	expectedSorted := sortStatuses(expectedOutput.Statuses)
	actualSorted := sortStatuses(actualStatuses)

	// Use custom comparison options to ignore timestamp differences and handle nil vs empty slice
	opts := []cmp.Option{
		cmpopts.EquateNaNs(),
		cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"),
		cmpopts.EquateEmpty(),
	}

	return cmp.Diff(expectedSorted, actualSorted, opts...), nil
}

func sortStatuses(statuses *Statuses) *Statuses {
	if statuses == nil {
		return nil
	}

	sorted := &Statuses{
		Gateways:     make(map[string]*gwv1.GatewayStatus),
		ListenerSets: make(map[string]*gwxv1.ListenerSetStatus),
		HTTPRoutes:   make(map[string]*gwv1.RouteStatus),
		TCPRoutes:    make(map[string]*gwv1.RouteStatus),
		TLSRoutes:    make(map[string]*gwv1.RouteStatus),
		GRPCRoutes:   make(map[string]*gwv1.RouteStatus),
		Policies:     make(map[string]*gwv1a2.PolicyStatus),
	}

	// Sort gateways
	gatewayKeys := make([]string, 0, len(statuses.Gateways))
	for k := range statuses.Gateways {
		gatewayKeys = append(gatewayKeys, k)
	}
	sort.Strings(gatewayKeys)
	for _, k := range gatewayKeys {
		sorted.Gateways[k] = statuses.Gateways[k]
	}

	// Sort listener sets
	listenerSetKeys := make([]string, 0, len(statuses.ListenerSets))
	for k := range statuses.ListenerSets {
		listenerSetKeys = append(listenerSetKeys, k)
	}
	sort.Strings(listenerSetKeys)
	for _, k := range listenerSetKeys {
		sorted.ListenerSets[k] = statuses.ListenerSets[k]
	}

	// Sort HTTP routes
	httpRouteKeys := make([]string, 0, len(statuses.HTTPRoutes))
	for k := range statuses.HTTPRoutes {
		httpRouteKeys = append(httpRouteKeys, k)
	}
	sort.Strings(httpRouteKeys)
	for _, k := range httpRouteKeys {
		sorted.HTTPRoutes[k] = statuses.HTTPRoutes[k]
	}

	// Sort TCP routes
	tcpRouteKeys := make([]string, 0, len(statuses.TCPRoutes))
	for k := range statuses.TCPRoutes {
		tcpRouteKeys = append(tcpRouteKeys, k)
	}
	sort.Strings(tcpRouteKeys)
	for _, k := range tcpRouteKeys {
		sorted.TCPRoutes[k] = statuses.TCPRoutes[k]
	}

	// Sort TLS routes
	tlsRouteKeys := make([]string, 0, len(statuses.TLSRoutes))
	for k := range statuses.TLSRoutes {
		tlsRouteKeys = append(tlsRouteKeys, k)
	}
	sort.Strings(tlsRouteKeys)
	for _, k := range tlsRouteKeys {
		sorted.TLSRoutes[k] = statuses.TLSRoutes[k]
	}

	// Sort GRPC routes
	grpcRouteKeys := make([]string, 0, len(statuses.GRPCRoutes))
	for k := range statuses.GRPCRoutes {
		grpcRouteKeys = append(grpcRouteKeys, k)
	}
	sort.Strings(grpcRouteKeys)
	for _, k := range grpcRouteKeys {
		sorted.GRPCRoutes[k] = statuses.GRPCRoutes[k]
	}

	// Sort policies
	policyKeys := make([]string, 0, len(statuses.Policies))
	for k := range statuses.Policies {
		policyKeys = append(policyKeys, k)
	}
	sort.Strings(policyKeys)
	for _, k := range policyKeys {
		sorted.Policies[k] = statuses.Policies[k]
	}

	return sorted
}
