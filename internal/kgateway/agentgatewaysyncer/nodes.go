package agentgatewaysyncer

import (
	"github.com/agentgateway/agentgateway/go/api"
	"istio.io/api/label"
	"istio.io/istio/pilot/pkg/util/protoconv"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
)

type Node struct {
	Name     string
	Locality *api.Locality
}

func (n Node) ResourceName() string {
	return n.Name
}

func (n Node) Equals(o Node) bool {
	return n.Name == o.Name &&
		protoconv.Equals(n.Locality, o.Locality)
}

// NodesCollection maps a node to it's locality.
// In many environments, nodes change frequently causing excessive recomputation of workloads.
// By making an intermediate collection we can reduce the times we need to trigger dependants (locality should ~never change).
func NodesCollection(nodes krt.Collection[*corev1.Node], opts ...krt.CollectionOption) krt.Collection[Node] {
	return krt.NewCollection(nodes, func(ctx krt.HandlerContext, k *corev1.Node) *Node {
		node := &Node{
			Name: k.Name,
		}
		region := k.GetLabels()[corev1.LabelTopologyRegion]
		zone := k.GetLabels()[corev1.LabelTopologyZone]
		subzone := k.GetLabels()[label.TopologySubzone.Name]

		if region != "" || zone != "" || subzone != "" {
			node.Locality = &api.Locality{
				Region:  region,
				Zone:    zone,
				Subzone: subzone,
			}
		}

		return node
	}, opts...)
}
