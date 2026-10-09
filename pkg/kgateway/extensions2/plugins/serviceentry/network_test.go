package serviceentry

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"istio.io/api/annotation"
	"istio.io/api/label"
	networking "istio.io/api/networking/v1alpha3"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/endpoints"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// TestSelectedWorkloadFromEntry_Network verifies a WorkloadEntry's network is
// resolved like Istio does: spec.network, else the network label, else the
// system namespace network that pods without a network label also resolve to.
func TestSelectedWorkloadFromEntry_Network(t *testing.T) {
	tests := []struct {
		name                   string
		metadataLabels         map[string]string
		specNetwork            string
		systemNamespaceNetwork string
		want                   string
	}{
		{
			name:                   "spec network wins",
			metadataLabels:         map[string]string{label.TopologyNetwork.Name: "from-label"},
			specNetwork:            "cluster2",
			systemNamespaceNetwork: "cluster1",
			want:                   "cluster2",
		},
		{
			name:                   "network label used without spec network",
			metadataLabels:         map[string]string{label.TopologyNetwork.Name: "cluster2"},
			systemNamespaceNetwork: "cluster1",
			want:                   "cluster2",
		},
		{
			name:                   "system namespace network used without spec network or label",
			systemNamespaceNetwork: "cluster1",
			want:                   "cluster1",
		},
		{
			name: "no network anywhere",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workload := selectedWorkloadFromEntry(
				"we", "ns",
				tt.metadataLabels,
				nil,
				nil,
				&networking.WorkloadEntry{Address: "1.2.3.4", Network: tt.specNetwork},
				nil,
				tt.systemNamespaceNetwork,
			)
			assert.Equal(t, tt.want, workload.network)
			nw, ok := workload.AugmentedLabels[label.TopologyNetwork.Name]
			assert.Equal(t, tt.want != "", ok, "network label presence")
			assert.Equal(t, tt.want, nw)
		})
	}
}

// A relabelled system namespace must re-emit inline ServiceEntry backends, since
// their endpoints carry the system namespace network.
func TestServiceEntryBackendIR_ReactsToSystemNamespaceNetwork(t *testing.T) {
	se := serviceEntryWithStatusAddrs(1)
	n1 := BuildServiceEntryBackendObjectIR(se, "server.server.mesh.internal", 80, "HTTP", nil, "n1").ObjIr
	n2 := BuildServiceEntryBackendObjectIR(se, "server.server.mesh.internal", 80, "HTTP", nil, "n2").ObjIr
	n1Again := BuildServiceEntryBackendObjectIR(se, "server.server.mesh.internal", 80, "HTTP", nil, "n1").ObjIr

	assert.False(t, n1.Equals(n2), "a system namespace network change must NOT be considered equal")
	assert.True(t, n1.Equals(n1Again), "the same system namespace network must be equal")
}

// TestBuildInlineEndpoints_SystemNamespaceNetwork verifies an unlabelled local inline
// endpoint inherits the system namespace network, so with PreferNetwork it ranks ahead
// of a remote endpoint for a gateway in the system namespace network instead of tying
// with it.
func TestBuildInlineEndpoints_SystemNamespaceNetwork(t *testing.T) {
	se := &networkingclient.ServiceEntry{
		Name:        "inlined-se",
		Namespace:   "gwtest",
		Annotations: map[string]string{annotation.NetworkingTrafficDistribution.Name: "PreferNetwork"},
		Spec: networking.ServiceEntry{
			Hosts:      []string{"se.example.com"},
			Location:   networking.ServiceEntry_MESH_INTERNAL,
			Resolution: networking.ServiceEntry_STATIC,
			Ports:      []*networking.ServicePort{{Name: "http", Number: 80, Protocol: "TCP"}},
			Endpoints: []*networking.WorkloadEntry{
				{Address: "1.1.1.1"}, // local, no network
				{Address: "2.2.2.2", Network: "n2"},
			},
		},
	}
	be := BuildServiceEntryBackendObjectIR(se, "se.example.com", 80, "TCP", nil, "n1")
	plugin := &serviceEntryPlugin{logger: slog.Default()}
	eps := plugin.buildInlineEndpoints(be, se)

	ucc := ir.NewUniquelyConnectedClient("gw", "gwtest", map[string]string{label.TopologyNetwork.Name: "n1"}, ir.PodLocality{})
	cla := endpoints.PrioritizeEndpoints(nil, ucc, endpoints.EndpointsInputs{EndpointsForBackend: *eps})

	priorities := map[string]uint32{}
	for _, group := range cla.GetEndpoints() {
		for _, lbEp := range group.GetLbEndpoints() {
			priorities[lbEp.GetEndpoint().GetAddress().GetSocketAddress().GetAddress()] = group.GetPriority()
		}
	}
	assert.Equal(t, map[string]uint32{"1.1.1.1": 0, "2.2.2.2": 1}, priorities)
}

// Like Istio, a DNS ServiceEntry without endpoints turns its hosts into
// endpoints that get no network, not the system namespace network.
func TestBuildInlineEndpoints_DNSHostsGetNoNetwork(t *testing.T) {
	se := &networkingclient.ServiceEntry{
		Name: "dns-se", Namespace: "gwtest",
		Spec: networking.ServiceEntry{
			Hosts:      []string{"se.example.com"},
			Location:   networking.ServiceEntry_MESH_EXTERNAL,
			Resolution: networking.ServiceEntry_DNS,
			Ports:      []*networking.ServicePort{{Name: "http", Number: 80, Protocol: "TCP"}},
		},
	}
	be := BuildServiceEntryBackendObjectIR(se, "se.example.com", 80, "TCP", nil, "n1")
	plugin := &serviceEntryPlugin{logger: slog.Default()}
	eps := plugin.buildInlineEndpoints(be, se)

	var count int
	for _, group := range eps.LbEps {
		for _, ep := range group {
			count++
			_, ok := ep.EndpointMd.Labels[label.TopologyNetwork.Name]
			assert.False(t, ok, "host endpoint %s must not get a network", ep.GetEndpoint().GetAddress().GetSocketAddress().GetAddress())
		}
	}
	assert.Equal(t, 1, count, "one endpoint per host")
}
