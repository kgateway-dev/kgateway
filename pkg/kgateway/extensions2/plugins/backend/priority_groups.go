package backend

import (
	"fmt"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoydnsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dns/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmputils"
)

// PriorityGroupsIr is the internal representation of a priority groups backend.
// The referenced static backends are flattened into a single load assignment
// with one locality per priority group, whose priority matches the group's
// position in the list.
type PriorityGroupsIr struct {
	// +noKrtEquals
	clusterTypeConfig *anypb.Any
	// +noKrtEquals
	loadAssignment *envoyendpointv3.ClusterLoadAssignment
}

// Equals checks if two PriorityGroupsIr objects are equal.
func (u *PriorityGroupsIr) Equals(other any) bool {
	otherPg, ok := other.(*PriorityGroupsIr)
	if !ok {
		return false
	}
	return cmputils.CompareWithNils(u, otherPg, func(a, b *PriorityGroupsIr) bool {
		return proto.Equal(a.clusterTypeConfig, b.clusterTypeConfig) &&
			proto.Equal(a.loadAssignment, b.loadAssignment)
	})
}

// buildPriorityGroupsIr resolves the backends referenced by each priority
// group and merges their endpoints into a single load assignment. Each group
// becomes one locality whose priority is the group's index, so Envoy only
// sends traffic to a group when the preceding groups are unhealthy.
// Only static backends may be referenced.
func buildPriorityGroupsIr(
	krtctx krt.HandlerContext,
	col krt.Collection[*kgateway.Backend],
	be *kgateway.Backend,
) (*PriorityGroupsIr, []error) {
	var errs []error
	needsDNS := false
	loadAssignment := &envoyendpointv3.ClusterLoadAssignment{}

	for gi, group := range be.Spec.PriorityGroups {
		locality := &envoyendpointv3.LocalityLbEndpoints{
			Priority: uint32(gi), //nolint:gosec // G115: group index is bounded by the CRD list length, always safe
		}
		for _, ref := range group.BackendRefs {
			refBe := krt.FetchOne(krtctx, col, krt.FilterKey(be.GetNamespace()+"/"+ref.Name))
			if refBe == nil {
				errs = append(errs, fmt.Errorf("priority group %d: backend %q not found in namespace %q", gi, ref.Name, be.GetNamespace()))
				continue
			}
			if (*refBe).Spec.Static == nil {
				errs = append(errs, fmt.Errorf("priority group %d: backend %q is not a static backend; only static backends are supported in priority groups", gi, ref.Name))
				continue
			}
			staticIr, err := buildStaticIr((*refBe).Spec.Static)
			if err != nil {
				errs = append(errs, fmt.Errorf("priority group %d: backend %q: %w", gi, ref.Name, err))
				continue
			}
			if staticIr.clusterTypeConfig != nil {
				needsDNS = true
			}
			for _, ep := range staticIr.loadAssignment.GetEndpoints() {
				locality.LbEndpoints = append(locality.LbEndpoints, ep.GetLbEndpoints()...)
			}
		}
		loadAssignment.Endpoints = append(loadAssignment.Endpoints, locality)
	}

	pgIr := &PriorityGroupsIr{
		loadAssignment: loadAssignment,
	}
	// at least one referenced backend has a DNS hostname, so the whole
	// cluster needs Envoy-side DNS resolution.
	if needsDNS {
		dnsClusterConfig, err := utils.MessageToAny(&envoydnsv3.DnsCluster{})
		if err != nil {
			return nil, append(errs, err)
		}
		pgIr.clusterTypeConfig = dnsClusterConfig
	}
	return pgIr, errs
}

// processPriorityGroups applies the priority groups IR to the envoy cluster.
func processPriorityGroups(ir *PriorityGroupsIr, out *envoyclusterv3.Cluster) {
	if ir.clusterTypeConfig != nil {
		out.ClusterDiscoveryType = &envoyclusterv3.Cluster_ClusterType{
			ClusterType: &envoyclusterv3.Cluster_CustomClusterType{
				Name:        dnsClusterExtensionName,
				TypedConfig: proto.Clone(ir.clusterTypeConfig).(*anypb.Any),
			},
		}
	} else {
		out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{
			Type: envoyclusterv3.Cluster_STATIC,
		}
	}

	if ir.loadAssignment != nil {
		// clone needed to avoid adding cluster name to original object in the IR.
		out.LoadAssignment = proto.Clone(ir.loadAssignment).(*envoyendpointv3.ClusterLoadAssignment)
		out.LoadAssignment.ClusterName = out.GetName()
	}
}
