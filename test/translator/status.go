package translator

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/backendtlspolicy"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	pluginreporter "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

type Statuses struct {
	Gateways     map[string]*gwv1.GatewayStatus     `json:"gateways,omitempty"`
	ListenerSets map[string]*gwv1.ListenerSetStatus `json:"listenerSets,omitempty"`
	HTTPRoutes   map[string]*gwv1.RouteStatus       `json:"httpRoutes,omitempty"`
	TCPRoutes    map[string]*gwv1.RouteStatus       `json:"tcpRoutes,omitempty"`
	TLSRoutes    map[string]*gwv1.RouteStatus       `json:"tlsRoutes,omitempty"`
	GRPCRoutes   map[string]*gwv1.RouteStatus       `json:"grpcRoutes,omitempty"`
	Policies     map[string]*gwv1.PolicyStatus      `json:"policies,omitempty"`
	Backends     map[string]*kgateway.BackendStatus `json:"backends,omitempty"`

	GatewayExtensions map[string]*kgateway.GatewayExtensionStatus `json:"gatewayExtensions,omitempty"`
}

func buildStatusesFromReports(
	reportsMap reports.ReportMap,
	gateways map[types.NamespacedName]*gwv1.Gateway,
	listenerSets map[types.NamespacedName]*gwv1.ListenerSet,
	gatewayExtensions map[string]*kgateway.GatewayExtensionStatus,
) *Statuses {
	// Fixed values for deterministic golden file tests. Use the zero time
	// for consistency and to avoid confusion about the significance of a
	// specific date.
	fixedTime := metav1.Time{Time: time.Time{}}

	statuses := &Statuses{
		Gateways:     make(map[string]*gwv1.GatewayStatus),
		ListenerSets: make(map[string]*gwv1.ListenerSetStatus),
		HTTPRoutes:   make(map[string]*gwv1.RouteStatus),
		TCPRoutes:    make(map[string]*gwv1.RouteStatus),
		TLSRoutes:    make(map[string]*gwv1.RouteStatus),
		GRPCRoutes:   make(map[string]*gwv1.RouteStatus),
		Policies:     make(map[string]*gwv1.PolicyStatus),
		Backends:     make(map[string]*kgateway.BackendStatus),

		GatewayExtensions: gatewayExtensions,
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
		if status := reportsMap.BuildGWStatus(gw, nil); status != nil {
			normalizeStatus(status, fixedTime)
			statuses.Gateways[gwNN.String()] = status
		}
	}

	// Build ListenerSet statuses. We need to use the actual ListenerSet object to make sure that
	// status.listeners are correctly populated instead of using the object metadata.
	for gvk, listenerSetsForGVK := range reportsMap.ListenerSets {
		for listenerSetNN := range listenerSetsForGVK {
			// Use the actual ListenerSet object from the input if available, otherwise create an empty one.
			var listenerSet gwv1.ListenerSet
			if actualLS, exists := listenerSets[listenerSetNN]; exists && actualLS != nil {
				listenerSet = *actualLS
			} else {
				listenerSet = gwv1.ListenerSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      listenerSetNN.Name,
						Namespace: listenerSetNN.Namespace,
					},
				}
			}
			if listenerSet.GroupVersionKind().Empty() {
				listenerSet.SetGroupVersionKind(gvk)
			}
			if status := reportsMap.BuildListenerSetStatus(listenerSet); status != nil {
				normalizeListenerSetStatus(status, fixedTime)
				statuses.ListenerSets[listenerSetNN.String()] = status
			}
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
		if status := reportsMap.BuildRouteStatus(&route, wellknown.DefaultGatewayClassName); status != nil {
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
		if status := reportsMap.BuildRouteStatus(&route, wellknown.DefaultGatewayClassName); status != nil {
			normalizeRouteStatus(status, fixedTime)
			statuses.TCPRoutes[routeNN.String()] = status
		}
	}

	// Build TLSRoute statuses
	for routeNN := range reportsMap.TLSRoutes {
		route := gwv1.TLSRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeNN.Name,
				Namespace: routeNN.Namespace,
			},
		}
		if status := reportsMap.BuildRouteStatus(&route, wellknown.DefaultGatewayClassName); status != nil {
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
		if status := reportsMap.BuildRouteStatus(&route, wellknown.DefaultGatewayClassName); status != nil {
			normalizeRouteStatus(status, fixedTime)
			statuses.GRPCRoutes[routeNN.String()] = status
		}
	}

	// Build Policy statuses
	for policyKey := range reportsMap.Policies {
		policyKeyStr := fmt.Sprintf("%s/%s/%s", policyKey.Kind, policyKey.Namespace, policyKey.Name)
		if status := buildPolicyStatus(reportsMap, policyKey, gwv1.PolicyStatus{}); status != nil {
			normalizePolicyStatus(status, fixedTime)
			statuses.Policies[policyKeyStr] = status
		}
	}

	// Build Backend statuses
	for backendNN := range reportsMap.Backends {
		backend := kgateway.Backend{
			ObjectMeta: metav1.ObjectMeta{
				Name:      backendNN.Name,
				Namespace: backendNN.Namespace,
			},
		}
		if status := reportsMap.BuildBackendStatus(&backend, kgateway.BackendStatus{}); status != nil {
			normalizeConditions(status.Conditions, fixedTime)
			statuses.Backends[backendNN.String()] = status
		}
	}

	return statuses
}

func buildPolicyStatus(
	reportsMap reports.ReportMap,
	policyKey pluginreporter.PolicyKey,
	currentStatus gwv1.PolicyStatus,
) *gwv1.PolicyStatus {
	// BackendTLSPolicy uses a custom desired-status builder (registered via the plugin's
	// RegisterPolicyStatus hook at runtime); mirror it here for golden outputs.
	if policyKey.Group == gwv1.GroupName && policyKey.Kind == wellknown.BackendTLSPolicyKind {
		pol := &gwv1.BackendTLSPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      policyKey.Name,
				Namespace: policyKey.Namespace,
			},
			Status: currentStatus,
		}
		return backendtlspolicy.BuildDesiredPolicyStatus(reportsMap.PolicyReport(policyKey), pol, wellknown.DefaultGatewayControllerName)
	}

	return reportsMap.BuildPolicyStatus(policyKey, wellknown.DefaultGatewayControllerName, currentStatus)
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
func normalizeListenerSetStatus(status *gwv1.ListenerSetStatus, time metav1.Time) {
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
func normalizePolicyStatus(status *gwv1.PolicyStatus, time metav1.Time) {
	for i := range status.Ancestors {
		for j := range status.Ancestors[i].Conditions {
			status.Ancestors[i].Conditions[j].LastTransitionTime = time
		}
	}
}

// normalizeConditions sets LastTransitionTime to a fixed value for deterministic testing.
func normalizeConditions(conditions []metav1.Condition, time metav1.Time) {
	for i := range conditions {
		conditions[i].LastTransitionTime = time
	}
}

// gatewayExtensionStatuses builds GatewayExtension statuses with the writer the plugins
// register at runtime, so golden files capture what the controller would write.
type gatewayExtensionStatuses struct {
	sources *statussync.StatusCollections
	writer  statussync.Writer[*kgateway.GatewayExtension, kgateway.GatewayExtensionStatus]
	found   bool
}

// registerGatewayExtensionStatuses runs the plugins' status registrations the way the proxy
// syncer does. Call it before the client starts, so the collections it builds are synced.
func registerGatewayExtensionStatuses(plugins pluginsdk.Plugin, krtOpts krtutil.KrtOptions) gatewayExtensionStatuses {
	g := gatewayExtensionStatuses{sources: statussync.NewStatusCollections()}
	plugins.RegisterStatus(statussync.RegistrationInputs{
		Collections: g.sources,
		KrtOpts:     krtOpts,
		RegisterWriter: func(_ schema.GroupVersionKind, syncer statussync.ResourceStatusSyncer) {
			if w, ok := syncer.(statussync.Writer[*kgateway.GatewayExtension, kgateway.GatewayExtensionStatus]); ok {
				g.writer, g.found = w, true
			}
		},
	})
	return g
}

func (g gatewayExtensionStatuses) HasSynced() bool {
	return g.sources.HasSynced()
}

// build returns the status written to each extension, starting from an empty status.
func (g gatewayExtensionStatuses) build(exts []ir.GatewayExtension) map[string]*kgateway.GatewayExtensionStatus {
	if !g.found {
		return nil
	}
	out := make(map[string]*kgateway.GatewayExtensionStatus, len(exts))
	for _, ext := range exts {
		nn := types.NamespacedName{Namespace: ext.Namespace, Name: ext.Name}
		res := statussync.Resource{GroupVersionKind: wellknown.GatewayExtensionGVK, NamespacedName: nn}
		obj := &kgateway.GatewayExtension{ObjectMeta: metav1.ObjectMeta{Namespace: nn.Namespace, Name: nn.Name}}
		if status, ok := g.writer.Desired(res, obj); ok {
			normalizeConditions(status.Conditions, metav1.Time{})
			out[nn.String()] = &status
		}
	}
	return out
}

func compareStatuses(expectedStatuses, actualStatuses *Statuses) string {
	if expectedStatuses == nil && actualStatuses == nil {
		return ""
	}
	if expectedStatuses == nil {
		return "expected no statuses but got some"
	}
	if actualStatuses == nil {
		return "expected statuses but got none"
	}

	// Sort statuses for consistent comparison
	expectedSorted := sortStatuses(expectedStatuses)
	actualSorted := sortStatuses(actualStatuses)

	// Use custom comparison options to ignore timestamp differences and handle nil vs empty slice
	opts := []cmp.Option{
		cmpopts.EquateNaNs(),
		cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"),
		cmpopts.EquateEmpty(),
	}

	return cmp.Diff(expectedSorted, actualSorted, opts...)
}

func sortStatuses(statuses *Statuses) *Statuses {
	if statuses == nil {
		return nil
	}

	sorted := &Statuses{
		Gateways:     make(map[string]*gwv1.GatewayStatus),
		ListenerSets: make(map[string]*gwv1.ListenerSetStatus),
		HTTPRoutes:   make(map[string]*gwv1.RouteStatus),
		TCPRoutes:    make(map[string]*gwv1.RouteStatus),
		TLSRoutes:    make(map[string]*gwv1.RouteStatus),
		GRPCRoutes:   make(map[string]*gwv1.RouteStatus),
		Policies:     make(map[string]*gwv1.PolicyStatus),
		Backends:     make(map[string]*kgateway.BackendStatus),
	}

	// Sort gateways
	gatewayKeys := make([]string, 0, len(statuses.Gateways))
	for k := range statuses.Gateways {
		gatewayKeys = append(gatewayKeys, k)
	}
	slices.Sort(gatewayKeys)
	for _, k := range gatewayKeys {
		sorted.Gateways[k] = statuses.Gateways[k]
	}

	// Sort listener sets
	listenerSetKeys := make([]string, 0, len(statuses.ListenerSets))
	for k := range statuses.ListenerSets {
		listenerSetKeys = append(listenerSetKeys, k)
	}
	slices.Sort(listenerSetKeys)
	for _, k := range listenerSetKeys {
		sorted.ListenerSets[k] = statuses.ListenerSets[k]
	}

	// Sort HTTP routes
	httpRouteKeys := make([]string, 0, len(statuses.HTTPRoutes))
	for k := range statuses.HTTPRoutes {
		httpRouteKeys = append(httpRouteKeys, k)
	}
	slices.Sort(httpRouteKeys)
	for _, k := range httpRouteKeys {
		sorted.HTTPRoutes[k] = statuses.HTTPRoutes[k]
	}

	// Sort TCP routes
	tcpRouteKeys := make([]string, 0, len(statuses.TCPRoutes))
	for k := range statuses.TCPRoutes {
		tcpRouteKeys = append(tcpRouteKeys, k)
	}
	slices.Sort(tcpRouteKeys)
	for _, k := range tcpRouteKeys {
		sorted.TCPRoutes[k] = statuses.TCPRoutes[k]
	}

	// Sort TLS routes
	tlsRouteKeys := make([]string, 0, len(statuses.TLSRoutes))
	for k := range statuses.TLSRoutes {
		tlsRouteKeys = append(tlsRouteKeys, k)
	}
	slices.Sort(tlsRouteKeys)
	for _, k := range tlsRouteKeys {
		sorted.TLSRoutes[k] = statuses.TLSRoutes[k]
	}

	// Sort GRPC routes
	grpcRouteKeys := make([]string, 0, len(statuses.GRPCRoutes))
	for k := range statuses.GRPCRoutes {
		grpcRouteKeys = append(grpcRouteKeys, k)
	}
	slices.Sort(grpcRouteKeys)
	for _, k := range grpcRouteKeys {
		sorted.GRPCRoutes[k] = statuses.GRPCRoutes[k]
	}

	// Sort policies
	policyKeys := make([]string, 0, len(statuses.Policies))
	for k := range statuses.Policies {
		policyKeys = append(policyKeys, k)
	}
	slices.Sort(policyKeys)
	for _, k := range policyKeys {
		sorted.Policies[k] = statuses.Policies[k]
	}

	// Sort backends
	backendKeys := make([]string, 0, len(statuses.Backends))
	for k := range statuses.Backends {
		backendKeys = append(backendKeys, k)
	}
	slices.Sort(backendKeys)
	for _, k := range backendKeys {
		sorted.Backends[k] = statuses.Backends[k]
	}

	sorted.GatewayExtensions = maps.Clone(statuses.GatewayExtensions)

	return sorted
}
