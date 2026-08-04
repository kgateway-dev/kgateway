package proxy_syncer

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

func addr(t gwv1.AddressType, value string) gwv1.GatewayStatusAddress {
	return gwv1.GatewayStatusAddress{Type: &t, Value: value}
}

// TestMergeGatewayStatusAddressesPreservesLiveOrder is the regression test for the
// status-syncer/deployer address flip-flop: the deployer decides whether to write using an
// order-sensitive slices.Equal against the live list, so we must never reorder it. A
// LoadBalancer whose ingress IPs are not in lexicographic order is the common trigger.
func TestMergeGatewayStatusAddressesPreservesLiveOrder(t *testing.T) {
	live := []gwv1.GatewayStatusAddress{
		addr(gwv1.IPAddressType, "10.0.0.9"),
		addr(gwv1.IPAddressType, "10.0.0.10"),
	}
	current := &gwv1.Gateway{Status: gwv1.GatewayStatus{Addresses: live}}

	merged := mergeGatewayStatusAddresses(current, gwv1.GatewayStatus{})

	require.Equal(t, live, merged.Addresses, "live address order must be preserved verbatim")
}

// TestMergeGatewayStatusAddressesPrefersLiveOverDesired ensures we write the freshest
// addresses from the informer rather than an earlier report snapshot, so a concurrent
// deployer address update is never reverted.
func TestMergeGatewayStatusAddressesPrefersLiveOverDesired(t *testing.T) {
	current := &gwv1.Gateway{Status: gwv1.GatewayStatus{Addresses: []gwv1.GatewayStatusAddress{
		addr(gwv1.IPAddressType, "1.1.1.1"),
		addr(gwv1.IPAddressType, "2.2.2.2"),
	}}}
	// Stale snapshot from when the status collection last recomputed.
	desired := gwv1.GatewayStatus{Addresses: []gwv1.GatewayStatusAddress{addr(gwv1.IPAddressType, "1.1.1.1")}}

	merged := mergeGatewayStatusAddresses(current, desired)

	require.Equal(t, current.Status.Addresses, merged.Addresses,
		"a stale desired snapshot must not revert the deployer's addresses")
}

// TestMergeGatewayStatusAddressesLeavesConditionsAlone guards against the merge touching
// anything other than addresses.
func TestMergeGatewayStatusAddressesLeavesConditionsAlone(t *testing.T) {
	conditions := []metav1.Condition{{
		Type:   string(gwv1.GatewayConditionProgrammed),
		Status: metav1.ConditionTrue,
		Reason: string(gwv1.GatewayReasonProgrammed),
	}}
	current := &gwv1.Gateway{Status: gwv1.GatewayStatus{
		Addresses:  []gwv1.GatewayStatusAddress{addr(gwv1.IPAddressType, "1.1.1.1")},
		Conditions: []metav1.Condition{},
	}}

	merged := mergeGatewayStatusAddresses(current, gwv1.GatewayStatus{Conditions: conditions})

	require.Equal(t, conditions, merged.Conditions, "conditions come from the desired status")
}

// TestBuildGWStatusCarriesLiveAddresses pins the status writer's assumption: BuildGWStatus
// copies live addresses through verbatim before the write-time merge refreshes them again.
func TestBuildGWStatusCarriesLiveAddresses(t *testing.T) {
	live := []gwv1.GatewayStatusAddress{
		addr(gwv1.IPAddressType, "10.0.0.9"),
		addr(gwv1.IPAddressType, "10.0.0.10"),
	}
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "kgateway",
			Listeners: []gwv1.Listener{{
				Name:     "http",
				Port:     80,
				Protocol: gwv1.HTTPProtocolType,
			}},
		},
		Status: gwv1.GatewayStatus{Addresses: live},
	}

	rm := reports.NewReportMap()
	reports.NewReporter(&rm).Gateway(gw)

	status := rm.BuildGWStatus(t.Context(), *gw, nil)
	require.NotNil(t, status)
	require.Equal(t, live, status.Addresses, "BuildGWStatus must carry live addresses through verbatim")
}

func TestChangedGatewayResourcesQueuesOnlyChangedReportKeys(t *testing.T) {
	unchanged := types.NamespacedName{Namespace: "default", Name: "unchanged"}
	changed := types.NamespacedName{Namespace: "default", Name: "changed"}
	old := reports.NewReportMap()
	old.HTTPRoutes[unchanged] = nil
	current := reports.NewReportMap()
	current.HTTPRoutes[unchanged] = nil
	current.HTTPRoutes[changed] = nil

	resources := changedGatewayResources(old, current, wellknown.TCPRouteV1GVR, wellknown.TLSRouteV1GVR)

	require.Equal(t, []types.NamespacedName{changed}, statusResourceNames(resources))
	require.Equal(t, wellknown.HTTPRouteGVK, resources[0].GroupVersionKind)
}

func TestChangedGatewayResourcesUsesServedRouteVersions(t *testing.T) {
	nn := types.NamespacedName{Namespace: "default", Name: "route"}
	old := reports.NewReportMap()
	current := reports.NewReportMap()
	current.TCPRoutes[nn] = nil
	current.TLSRoutes[nn] = nil

	resources := changedGatewayResources(old, current, wellknown.TCPRouteV1GVR, wellknown.TLSRouteV1Alpha3GVR)

	require.ElementsMatch(t, []schema.GroupVersionKind{
		wellknown.TCPRouteV1GVK,
		wellknown.TLSRouteV1Alpha3GVK,
	}, resourceGVKs(resources))
}

func TestChangedPolicyResourcesMapsPolicyGroupKindToWriterGVK(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "gateway.example.io", Version: "v1", Kind: "ExamplePolicy"}
	key := reporter.PolicyKey{Group: gvk.Group, Kind: gvk.Kind, Namespace: "default", Name: "policy"}
	old := reports.NewReportMap()
	current := reports.NewReportMap()
	current.Policies[key] = nil

	resources := changedPolicyResources(old, current, map[schema.GroupKind]schema.GroupVersionKind{gvk.GroupKind(): gvk})

	require.Equal(t, []types.NamespacedName{{Namespace: "default", Name: "policy"}}, statusResourceNames(resources))
	require.Equal(t, gvk, resources[0].GroupVersionKind)
}

func statusResourceNames(resources []statussync.Resource) []types.NamespacedName {
	result := make([]types.NamespacedName, 0, len(resources))
	for _, resource := range resources {
		result = append(result, resource.NamespacedName)
	}
	return result
}

func resourceGVKs(resources []statussync.Resource) []schema.GroupVersionKind {
	result := make([]schema.GroupVersionKind, 0, len(resources))
	for _, resource := range resources {
		result = append(result, resource.GroupVersionKind)
	}
	return result
}
