package destrule

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/waypoint"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestRemoteWaypointThenDestinationRule(t *testing.T) {
	backend := drBackend()
	backend.Obj = &corev1.Service{Spec: corev1.ServiceSpec{ClusterIP: "10.0.0.1", ClusterIPs: []string{"10.0.0.1"}}}
	out := &envoyclusterv3.Cluster{Name: backend.ClusterName()}
	// internal.kgateway.dev sorts before networking.istio.io: remote redirect first.
	waypoint.ApplyIngressUseWaypointCluster(backend, out, &apisettings.Settings{})
	require.Len(t, out.GetLoadAssignment().GetEndpoints(), 1)
	d := newDestrulePlugin(t, destRule("dr", &v1alpha3.TrafficPolicy{OutlierDetection: &v1alpha3.OutlierDetection{}, LoadBalancer: &v1alpha3.LoadBalancerSettings{LocalityLbSetting: &v1alpha3.LocalityLoadBalancerSetting{}}}))
	overlay := d.clusterOverlay(krt.TestingDummyContext{}, t.Context(), ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{}), backend)
	require.NotNil(t, overlay)
	overlay.Mutate(out)
	require.Nil(t, out.GetCommonLbConfig().GetLocalityConfigSpecifier(), "remote waypoint VIP has no locality weights")
}

func TestDestinationRuleRetainsWeightedStaticLocalities(t *testing.T) {
	out := &envoyclusterv3.Cluster{ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STATIC}, LoadAssignment: &envoyendpointv3.ClusterLoadAssignment{
		Endpoints: []*envoyendpointv3.LocalityLbEndpoints{{LoadBalancingWeight: wrapperspb.UInt32(1)}},
	}}
	applyLocalityLbConfig(&v1alpha3.TrafficPolicy{LoadBalancer: &v1alpha3.LoadBalancerSettings{LocalityLbSetting: &v1alpha3.LocalityLoadBalancerSetting{}}}, out)
	require.NotNil(t, out.GetCommonLbConfig().GetLocalityWeightedLbConfig())
}
