package proxy_syncer

import (
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/proto"
)

// clusterLoadAssignmentsEqual is proto.Equal for ClusterLoadAssignments that
// short-circuits on shared sub-messages. PrioritizeEndpoints builds every
// client's CLA from the same endpoint IR, so two candidates in one interning
// bucket usually reference the very same *LbEndpoint protos; the pinned
// protobuf-go's proto.Equal checks pointer identity only at the root and
// otherwise walks every field of every endpoint, which made the interner's
// equality check a second full pass over the endpoint tree per client per
// backend on the reconnect fan-out path.
//
// The fast path handles the shape PrioritizeEndpoints emits: cluster name plus
// locality groups carrying locality, priority, weight and endpoints. Anything
// outside that shape (Policy, NamedEndpoints, per-locality metadata, proximity,
// an LbConfig oneof, unknown fields) falls back to proto.Equal for the message
// that carries it, so the result agrees with proto.Equal on every input.
// TestClusterLoadAssignmentsEqualAgreesWithProtoEqual pins that agreement.
func clusterLoadAssignmentsEqual(a, b *envoyendpointv3.ClusterLoadAssignment) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if hasExtraCLAFields(a) || hasExtraCLAFields(b) {
		return proto.Equal(a, b)
	}
	if a.GetClusterName() != b.GetClusterName() {
		return false
	}
	if len(a.GetEndpoints()) != len(b.GetEndpoints()) {
		return false
	}
	for i := range a.GetEndpoints() {
		if !localityLbEndpointsEqual(a.GetEndpoints()[i], b.GetEndpoints()[i]) {
			return false
		}
	}
	return true
}

func hasExtraCLAFields(c *envoyendpointv3.ClusterLoadAssignment) bool {
	return c.GetPolicy() != nil ||
		len(c.GetNamedEndpoints()) > 0 ||
		len(c.ProtoReflect().GetUnknown()) > 0
}

// localityLbEndpointsEqual compares one locality group, treating an LbEndpoint
// pointer shared by both sides as equal without walking it.
func localityLbEndpointsEqual(a, b *envoyendpointv3.LocalityLbEndpoints) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if hasExtraLocalityFields(a) || hasExtraLocalityFields(b) {
		return proto.Equal(a, b)
	}
	if a.GetPriority() != b.GetPriority() {
		return false
	}
	if (a.GetLoadBalancingWeight() == nil) != (b.GetLoadBalancingWeight() == nil) ||
		a.GetLoadBalancingWeight().GetValue() != b.GetLoadBalancingWeight().GetValue() {
		return false
	}
	if !proto.Equal(a.GetLocality(), b.GetLocality()) {
		return false
	}
	if len(a.GetLbEndpoints()) != len(b.GetLbEndpoints()) {
		return false
	}
	for i := range a.GetLbEndpoints() {
		x, y := a.GetLbEndpoints()[i], b.GetLbEndpoints()[i]
		if x == y {
			continue
		}
		if !proto.Equal(x, y) {
			return false
		}
	}
	return true
}

func hasExtraLocalityFields(l *envoyendpointv3.LocalityLbEndpoints) bool {
	if l.GetMetadata() != nil || l.GetProximity() != nil || l.GetLbConfig() != nil {
		return true
	}
	if w := l.GetLoadBalancingWeight(); w != nil && len(w.ProtoReflect().GetUnknown()) > 0 {
		return true
	}
	return len(l.ProtoReflect().GetUnknown()) > 0
}
