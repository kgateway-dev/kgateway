package endpoints

import (
	"maps"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// TestLoadBalancingContextHashSoundness locks the coupling between
// LoadBalancingContextHash and PrioritizeEndpoints, which the CLA interning in
// NewPerClientEnvoyEndpoints relies on. The interning shares one CLA across every
// UCC with the same hash, so the hash MUST capture every UCC-dependent input that
// PrioritizeEndpoints consumes. We assert the soundness direction over a diverse
// UCC set and several priority configurations:
//
//	equal hash  =>  proto.Equal on the built ClusterLoadAssignment
//
// The reverse does NOT hold and is intentionally not asserted: the hash is
// conservative (e.g. single-group locality failover renormalizes every priority
// to 0, so UCCs in different localities can hash differently yet build identical
// CLAs). That only costs a missed dedup, never a wrong one. A future change to
// PrioritizeEndpoints that reads a UCC field not folded into the hash would break
// this test by producing equal-hash UCCs with differing CLAs.
//
// The CLAs are compared as built, with no normalization: PrioritizeEndpoints emits
// locality groups in canonical order (see sortedLocalities), which
// TestPrioritizeEndpointsIsByteStable pins independently. That coupling means a
// locality-ordering regression also fails here, which the failure message calls
// out — check the byte-stability test before hunting for a missing hash input.
func TestLoadBalancingContextHashSoundness(t *testing.T) {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{
		Group:     "core",
		Kind:      "Service",
		Namespace: "ns",
		Name:      "svc",
	}, 80, "", "")

	// Endpoints spread across localities, each labeled with its own topology so
	// failover-priority (which compares proxy labels to endpoint labels) and
	// locality failover both have something to discriminate on.
	ep := ir.NewEndpointsForBackend(backend)
	addEndpoint(ep, "r1", "z1", "ep-z1")
	addEndpoint(ep, "r1", "z2", "ep-z2")
	addEndpoint(ep, "r2", "z3", "ep-r2")

	uccs := []ir.UniquelyConnectedClient{
		// Same topology labels + locality, differ only in an irrelevant label:
		// must collapse to one hash and one CLA in every scenario.
		newUCC("z1-a", "r1", "z1", map[string]string{"app": "a"}),
		newUCC("z1-b", "r1", "z1", map[string]string{"app": "b"}),
		newUCC("z2", "r1", "z2", nil),
		newUCC("r2", "r2", "z3", nil),
		// No topology labels at all (locality still r1/z1): exercises the empty
		// label-value path of the failover-priority hash.
		newUCCNoTopology("no-topo", "r1", "z1"),
	}

	scenarios := map[string]EndpointsInputs{
		// PriorityInfo nil (TrafficDistribution Any): output is UCC-independent,
		// so every UCC hashes to 0 and builds an identical CLA.
		"trafficAny": {EndpointsForBackend: *ep},
		// Failover priority on topology labels: hash + CLA key on the resolved
		// proxy label values.
		"failoverPriority": {
			EndpointsForBackend: *ep,
			PriorityInfo: &PriorityInfo{
				FailoverPriority: NewPriorities([]string{corev1.LabelZoneRegion, corev1.LabelTopologyZone}),
			},
		},
		// Locality failover (no FailoverPriority): hash + CLA key on PodLocality.
		"localityFailover": {
			EndpointsForBackend: *ep,
			PriorityInfo:        &PriorityInfo{},
		},
	}

	for name, inputs := range scenarios {
		t.Run(name, func(t *testing.T) {
			hashes := make([]uint64, len(uccs))
			claList := make([]*envoyendpointv3.ClusterLoadAssignment, len(uccs))
			distinctHashes := map[uint64]struct{}{}
			for i, ucc := range uccs {
				hashes[i] = LoadBalancingContextHash(ucc, inputs)
				claList[i] = PrioritizeEndpoints(nil, ucc, inputs)
				distinctHashes[hashes[i]] = struct{}{}
			}

			// Soundness: equal hash must imply identical CLA.
			for i := range uccs {
				for j := i + 1; j < len(uccs); j++ {
					if hashes[i] == hashes[j] {
						require.True(t, proto.Equal(claList[i], claList[j]),
							"UCCs %q and %q share hash %d but built different CLAs: either the hash misses a "+
								"UCC-dependent input, or PrioritizeEndpoints is no longer emitting locality "+
								"groups in canonical order (check TestPrioritizeEndpointsIsByteStable first)",
							uccs[i].ResourceName(), uccs[j].ResourceName(), hashes[i])
					}
				}
			}

			// Sanity: the discriminating scenarios must actually produce more than
			// one hash, otherwise the soundness check above is vacuous.
			if name == "trafficAny" {
				require.Len(t, distinctHashes, 1, "UCC-independent scenario should yield a single hash")
			} else {
				require.Greater(t, len(distinctHashes), 1, "scenario %q did not discriminate between UCCs", name)
			}
		})
	}
}

func newUCC(role, region, zone string, extra map[string]string) ir.UniquelyConnectedClient {
	labels := map[string]string{
		corev1.LabelZoneRegion:   region,
		corev1.LabelTopologyZone: zone,
	}
	maps.Copy(labels, extra)
	return ir.NewUniquelyConnectedClient(role, "ns", labels, ir.PodLocality{Region: region, Zone: zone})
}

func newUCCNoTopology(role, region, zone string) ir.UniquelyConnectedClient {
	return ir.NewUniquelyConnectedClient(role, "ns", map[string]string{"app": role}, ir.PodLocality{Region: region, Zone: zone})
}

func addEndpoint(ep *ir.EndpointsForBackend, region, zone, path string) {
	ep.Add(ir.PodLocality{Region: region, Zone: zone}, ir.EndpointWithMd{
		LbEndpoint: &envoyendpointv3.LbEndpoint{
			HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
				Endpoint: &envoyendpointv3.Endpoint{
					Address: &envoycorev3.Address{
						Address: &envoycorev3.Address_Pipe{Pipe: &envoycorev3.Pipe{Path: path}},
					},
				},
			},
		},
		EndpointMd: ir.EndpointMetadata{
			Labels: map[string]string{
				corev1.LabelZoneRegion:   region,
				corev1.LabelTopologyZone: zone,
			},
		},
	})
}
