package proxy_syncer

import (
	"testing"

	"github.com/onsi/gomega"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

func TestNewPerClientLocalClusterEndpointsBuildsGatewayLocalities(t *testing.T) {
	g := gomega.NewWithT(t)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "ns", map[string]string{
		wellknown.GatewayNameLabel: "gw",
	}, ir.PodLocality{Region: "region-1", Zone: "zone-a"})
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})
	pods := krt.NewStaticCollection[krtcollections.LocalityPod](nil, []krtcollections.LocalityPod{
		{
			Named: krt.Named{
				Namespace: "ns",
				Name:      "gw-zone-a",
			},
			Locality: ir.PodLocality{Region: "region-1", Zone: "zone-a"},
			AugmentedLabels: map[string]string{
				wellknown.GatewayNameLabel: "gw",
			},
			Addresses: []string{"10.0.0.1"},
		},
		{
			Named: krt.Named{
				Namespace: "ns",
				Name:      "gw-zone-b",
			},
			Locality: ir.PodLocality{Region: "region-1", Zone: "zone-b"},
			AugmentedLabels: map[string]string{
				wellknown.GatewayNameLabel: "gw",
			},
			Addresses: []string{"10.0.0.2"},
		},
		{
			Named: krt.Named{
				Namespace: "ns",
				Name:      "gw-zone-a-2",
			},
			Locality: ir.PodLocality{Region: "region-1", Zone: "zone-a"},
			AugmentedLabels: map[string]string{
				wellknown.GatewayNameLabel: "gw",
			},
			Addresses: []string{"10.0.0.4"},
		},
		{
			Named: krt.Named{
				Namespace: "ns",
				Name:      "other-gw",
			},
			Locality: ir.PodLocality{Region: "region-1", Zone: "zone-c"},
			AugmentedLabels: map[string]string{
				wellknown.GatewayNameLabel: "other",
			},
			Addresses: []string{"10.0.0.3"},
		},
	})

	localEndpoints := NewPerClientLocalClusterEndpoints(krtutil.KrtOptions{}, uccs, pods)

	var got []UccWithEndpoints
	g.Eventually(func() []UccWithEndpoints {
		got = localEndpoints.endpoints.List()
		return got
	}).Should(gomega.HaveLen(1))

	cla := got[0].Endpoints
	g.Expect(cla.GetClusterName()).To(gomega.Equal("gw.ns"))
	g.Expect(cla.GetEndpoints()).To(gomega.HaveLen(2))

	addressesByZone := map[string][]string{}
	weightsByZone := map[string]uint32{}
	for _, localityEndpoints := range cla.GetEndpoints() {
		zone := localityEndpoints.GetLocality().GetZone()
		weightsByZone[zone] = localityEndpoints.GetLoadBalancingWeight().GetValue()
		for _, lbEndpoint := range localityEndpoints.GetLbEndpoints() {
			address := lbEndpoint.GetEndpoint().GetAddress().GetSocketAddress().GetAddress()
			addressesByZone[zone] = append(addressesByZone[zone], address)
			g.Expect(lbEndpoint.GetLoadBalancingWeight().GetValue()).To(gomega.Equal(uint32(1)))
		}
	}
	g.Expect(addressesByZone).To(gomega.Equal(map[string][]string{
		"zone-a": {"10.0.0.1", "10.0.0.4"},
		"zone-b": {"10.0.0.2"},
	}))
	g.Expect(weightsByZone).To(gomega.Equal(map[string]uint32{
		"zone-a": 2,
		"zone-b": 1,
	}))
}

func TestNewPerClientLocalClusterEndpointsUsesSafeClusterNameForLongGateways(t *testing.T) {
	g := gomega.NewWithT(t)

	longGatewayName := "gateway-with-a-name-that-is-longer-than-the-kubernetes-label-limit-and-needs-hashing"
	safeGatewayName := kubeutils.SafeGatewayLabelValue(longGatewayName)
	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", longGatewayName)
	ucc := ir.NewUniquelyConnectedClient(role, "ns", map[string]string{
		wellknown.GatewayNameAnnotation: longGatewayName,
		wellknown.GatewayNameLabel:      safeGatewayName,
	}, ir.PodLocality{Region: "region-1", Zone: "zone-a"})
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})
	pods := krt.NewStaticCollection[krtcollections.LocalityPod](nil, []krtcollections.LocalityPod{
		{
			Named: krt.Named{
				Namespace: "ns",
				Name:      "gw-zone-a",
			},
			Locality: ir.PodLocality{Region: "region-1", Zone: "zone-a"},
			AugmentedLabels: map[string]string{
				wellknown.GatewayNameAnnotation: longGatewayName,
				wellknown.GatewayNameLabel:      safeGatewayName,
			},
			Addresses: []string{"10.0.0.1"},
		},
	})

	localEndpoints := NewPerClientLocalClusterEndpoints(krtutil.KrtOptions{}, uccs, pods)

	var got []UccWithEndpoints
	g.Eventually(func() []UccWithEndpoints {
		got = localEndpoints.endpoints.List()
		return got
	}).Should(gomega.HaveLen(1))

	g.Expect(got[0].Endpoints.GetClusterName()).To(gomega.Equal(safeGatewayName + ".ns"))
}
