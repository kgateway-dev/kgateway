package proxy_syncer

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

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
// addresses from the informer rather than the snapshot the status collection computed with,
// so a concurrent deployer address update is never reverted.
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

// TestBuildGWStatusCarriesLiveAddresses pins the assumption the Gateway status collection
// relies on: BuildGWStatus copies the live addresses through verbatim, so the desired status
// matches the live one and the live-vs-desired comparison stays quiet. If this ever changes,
// the collection needs to carry addresses itself again.
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
