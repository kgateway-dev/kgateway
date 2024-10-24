package krtcollections

import (
	"context"
	"maps"

	"istio.io/api/label"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type NodeMetadata struct {
	name   string
	labels map[string]string
}

type PodLocality struct {
	Region  string
	Zone    string
	Subzone string
}

func (c NodeMetadata) ResourceName() string {
	return c.name
}

func (c NodeMetadata) Equals(in NodeMetadata) bool {
	return c.name == in.name && maps.Equal(c.labels, in.labels)
}

var _ krt.ResourceNamer = NodeMetadata{}
var _ krt.Equaler[NodeMetadata] = NodeMetadata{}

type LocalityPod struct {
	krt.Named
	Locality        PodLocality
	AugmentedLabels map[string]string
}

func (c LocalityPod) Equals(in LocalityPod) bool {
	return c.Named == in.Named && c.Locality == in.Locality && maps.Equal(c.AugmentedLabels, in.AugmentedLabels)
}

func newNodeCollection(istioClient kube.Client) krt.Collection[NodeMetadata] {
	nodeClient := kclient.New[*corev1.Node](istioClient)
	nodes := krt.WrapClient(nodeClient, krt.WithName("Nodes"))
	return NewNodeMetadataCollection(nodes)
}

func NewNodeMetadataCollection(nodes krt.Collection[*corev1.Node]) krt.Collection[NodeMetadata] {
	return krt.NewCollection(nodes, func(kctx krt.HandlerContext, us *corev1.Node) *NodeMetadata {
		return &NodeMetadata{
			name:   us.Name,
			labels: us.Labels,
		}
	})
}

func NewPodsCollection(ctx context.Context, istioClient kube.Client) krt.Collection[LocalityPod] {
	podClient := kclient.New[*corev1.Pod](istioClient)
	pods := krt.WrapClient(podClient, krt.WithName("Pods"))
	nodes := newNodeCollection(istioClient)
	return NewLocalityPodsCollection(nodes, pods)
}

func NewLocalityPodsCollection(nodes krt.Collection[NodeMetadata], pods krt.Collection[*corev1.Pod]) krt.Collection[LocalityPod] {
	return krt.NewCollection(pods, augmentPodLabels(nodes))
}

func augmentPodLabels(nodes krt.Collection[NodeMetadata]) func(kctx krt.HandlerContext, pod *corev1.Pod) *LocalityPod {
	return func(kctx krt.HandlerContext, pod *corev1.Pod) *LocalityPod {
		labels := maps.Clone(pod.Labels)
		if labels == nil {
			labels = make(map[string]string)
		}
		nodeName := pod.Spec.NodeName
		var l PodLocality
		if nodeName != "" {
			maybeNode := krt.FetchOne(kctx, nodes, krt.FilterObjectName(types.NamespacedName{
				Name: nodeName,
			}))
			if maybeNode != nil {
				node := *maybeNode
				nodeLabels := node.labels
				region := nodeLabels[corev1.LabelTopologyRegion]
				zone := nodeLabels[corev1.LabelTopologyZone]
				subzone := nodeLabels[label.TopologySubzone.Name]
				l = PodLocality{
					Region:  region,
					Zone:    zone,
					Subzone: subzone,
				}

				// augment labels
				if region != "" {
					labels[corev1.LabelTopologyRegion] = region
				}
				if zone != "" {
					labels[corev1.LabelTopologyZone] = zone
				}
				if subzone != "" {
					labels[label.TopologySubzone.Name] = subzone
				}
				//	labels[label.TopologyCluster.Name] = clusterID.String()
				//	labels[LabelHostname] = k8sNode
				//	labels[label.TopologyNetwork.Name] = networkID.String()
			}
		}

		return &LocalityPod{
			Named:           krt.NewNamed(pod),
			AugmentedLabels: labels,
			Locality:        l,
		}
	}

}
