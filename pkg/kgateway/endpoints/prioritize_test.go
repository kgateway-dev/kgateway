package endpoints

import (
	"strconv"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// TestPrioritizeEndpointsIsByteStable pins the determinism of inline-CLA cluster
// versions. The proxy syncer hashes the full cluster into ClusterVersion, which
// contributes to the CDS version. Ranging LbEps directly would put the same CLA
// localities in a fresh order, spuriously changing CDS and re-warming the cluster.
// EDS KRT equality uses the structural endpoint hash and does not depend on these
// serialized bytes.
//
// Every priority mode is covered because each takes a different path through
// getEndpoints and applyLocalityFailover.
func TestPrioritizeEndpointsIsByteStable(t *testing.T) {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{
		Group: "networking.istio.io", Kind: "ServiceEntry", Namespace: "ns", Name: "se",
	}, 80, "", "")
	backendEndpoints := ir.NewEndpointsForBackend(backend)
	// Enough localities that map ordering is overwhelmingly unlikely to be
	// stable by chance, plus the zero locality, which is emitted with a nil
	// Locality and so sorts ahead of everything else.
	localities := []ir.PodLocality{
		{},
		{Region: "r1", Zone: "z1"},
		{Region: "r1", Zone: "z2"},
		{Region: "r2", Zone: "z3"},
		{Region: "r3", Zone: "z4"},
	}
	for i, locality := range localities {
		backendEndpoints.Add(locality, prioritizeTestEndpoint(locality, i))
	}

	client := ir.NewUniquelyConnectedClient("role", "ns", map[string]string{
		corev1.LabelTopologyRegion: "r1",
		corev1.LabelTopologyZone:   "z1",
	}, ir.PodLocality{Region: "r1", Zone: "z1"})

	priorityModes := map[string]*PriorityInfo{
		// UCC-independent: no prioritization is applied at all.
		"noPriorityInfo": nil,
		// Groups endpoints by how well their topology labels match the client's.
		"failoverPriority": {
			FailoverPriority: NewPriorities([]string{corev1.LabelTopologyRegion, corev1.LabelTopologyZone}),
		},
		// Renormalizes priorities from the client's locality instead.
		"localityFailover": {},
	}

	for name, priorityInfo := range priorityModes {
		t.Run(name, func(t *testing.T) {
			inputs := EndpointsInputs{EndpointsForBackend: *backendEndpoints, PriorityInfo: priorityInfo}

			clusterVersions := map[uint64]struct{}{}
			for range 200 {
				cluster := &envoyclusterv3.Cluster{
					Name:           "inline-cluster",
					LoadAssignment: PrioritizeEndpoints(nil, client, inputs),
				}
				clusterVersions[utils.HashProto(cluster)] = struct{}{}
			}
			require.Len(t, clusterVersions, 1,
				"identical inputs must produce one inline-cluster version; got %d", len(clusterVersions))

			// Pin the canonical order too, so a change that is stable but no
			// longer sorted is a deliberate decision rather than a silent one.
			// Failover priority splits one locality into several groups, so
			// equal neighbors are expected.
			cla := PrioritizeEndpoints(nil, client, inputs)
			require.NotEmpty(t, cla.GetEndpoints())
			for i := 1; i < len(cla.GetEndpoints()); i++ {
				assert.LessOrEqual(t, localityOrderKey(cla.GetEndpoints()[i-1]), localityOrderKey(cla.GetEndpoints()[i]),
					"localities must be emitted in ascending (region, zone, subzone) order")
			}
		})
	}
}

// localityOrderKey renders a locality group's locality for ordering assertions. A
// nil Locality (the zero PodLocality) yields the empty key, which sorts first.
func localityOrderKey(group *envoyendpointv3.LocalityLbEndpoints) string {
	locality := group.GetLocality()
	return locality.GetRegion() + "/" + locality.GetZone() + "/" + locality.GetSubZone()
}

func prioritizeTestEndpoint(locality ir.PodLocality, index int) ir.EndpointWithMd {
	endpoint := editorTestEndpoint("10.0.0."+strconv.Itoa(index+1), "ep")
	endpoint.EndpointMd.Labels = map[string]string{
		corev1.LabelTopologyRegion: locality.Region,
		corev1.LabelTopologyZone:   locality.Zone,
	}
	return endpoint
}
