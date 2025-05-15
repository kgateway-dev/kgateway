package krtcollections

import (
	"maps"

	istioannot "istio.io/api/annotation"
	"istio.io/api/label"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/slices"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
)

type NodeMetadata struct {
	name   string
	labels map[string]string
}

func (c NodeMetadata) ResourceName() string {
	return c.name
}

func (c NodeMetadata) Equals(in NodeMetadata) bool {
	return c.name == in.name && maps.Equal(c.labels, in.labels)
}

var (
	_ krt.ResourceNamer         = NodeMetadata{}
	_ krt.Equaler[NodeMetadata] = NodeMetadata{}
)

type LocalityPod struct {
	krt.Named
	Locality        ir.PodLocality
	AugmentedLabels map[string]string
	Addresses       []string
}

// Addresses returns the first address if there are any.
func (c LocalityPod) Address() string {
	if len(c.Addresses) == 0 {
		return ""
	}
	return c.Addresses[0]
}

func (c LocalityPod) Equals(in LocalityPod) bool {
	return c.Named == in.Named &&
		c.Locality == in.Locality &&
		maps.Equal(c.AugmentedLabels, in.AugmentedLabels) &&
		slices.Equal(c.Addresses, in.Addresses)
}

func newNodeCollection(istioClient kube.Client, krtOptions krtutil.KrtOptions) krt.Collection[NodeMetadata] {
	nodeClient := kclient.NewFiltered[*corev1.Node](
		istioClient,
		kclient.Filter{ObjectFilter: istioClient.ObjectFilter()},
	)
	nodes := krt.WrapClient(nodeClient, krtOptions.ToOptions("Nodes")...)
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

func NewPodsCollection(istioClient kube.Client, krtOptions krtutil.KrtOptions) krt.Collection[LocalityPod] {
	podClient := kclient.NewFiltered[*corev1.Pod](istioClient, kclient.Filter{
		ObjectTransform: kube.StripPodUnusedFields,
		ObjectFilter:    istioClient.ObjectFilter(),
	})
	pods := krt.WrapClient(podClient, krtOptions.ToOptions("Pods")...)
	nodes := newNodeCollection(istioClient, krtOptions)
	return NewLocalityPodsCollection(nodes, pods, krtOptions)
}

func NewLocalityPodsCollection(nodes krt.Collection[NodeMetadata], pods krt.Collection[*corev1.Pod], krtOptions krtutil.KrtOptions) krt.Collection[LocalityPod] {
	return krt.NewCollection(pods, augmentPodLabels(nodes), krtOptions.ToOptions("AugmentPod")...)
}

func augmentPodLabels(nodes krt.Collection[NodeMetadata]) func(kctx krt.HandlerContext, pod *corev1.Pod) *LocalityPod {
	return func(kctx krt.HandlerContext, pod *corev1.Pod) *LocalityPod {
		labels := maps.Clone(pod.Labels)
		if labels == nil {
			labels = make(map[string]string)
		}
		nodeName := pod.Spec.NodeName
		var l ir.PodLocality
		if nodeName != "" {
			maybeNode := krt.FetchOne(kctx, nodes, krt.FilterObjectName(types.NamespacedName{
				Name: nodeName,
			}))
			if maybeNode != nil {
				node := *maybeNode
				nodeLabels := node.labels
				l = LocalityFromLabels(nodeLabels)
				AugmentLabels(l, labels)

				//	labels[label.TopologyCluster.Name] = clusterID.String()
				//	labels[LabelHostname] = k8sNode
				//	labels[label.TopologyNetwork.Name] = networkID.String()
			}
		}

		// Augment the labels with the ambient redirection annotation
		if redirectionValue, exists := pod.Annotations[istioannot.AmbientRedirection.Name]; exists {
			labels[istioannot.AmbientRedirection.Name] = redirectionValue
		}

		return &LocalityPod{
			Named:           krt.NewNamed(pod),
			AugmentedLabels: labels,
			Locality:        l,
			Addresses:       extractPodIPs(pod),
		}
	}
}

func LocalityFromLabels(labels map[string]string) ir.PodLocality {
	region := labels[corev1.LabelTopologyRegion]
	zone := labels[corev1.LabelTopologyZone]
	subzone := labels[label.TopologySubzone.Name]
	return ir.PodLocality{
		Region:  region,
		Zone:    zone,
		Subzone: subzone,
	}
}

func AugmentLabels(locality ir.PodLocality, labels map[string]string) {
	// augment labels
	if locality.Region != "" {
		labels[corev1.LabelTopologyRegion] = locality.Region
	}
	if locality.Zone != "" {
		labels[corev1.LabelTopologyZone] = locality.Zone
	}
	if locality.Subzone != "" {
		labels[label.TopologySubzone.Name] = locality.Subzone
	}
}

// technically the plural PodIPs isn't a required field.
// we don't use it yet, but it will be useful to support ipv6
// "Pods may be allocated at most 1 value for each of IPv4 and IPv6."
//   - k8s docs
func extractPodIPs(pod *corev1.Pod) []string {
	if len(pod.Status.PodIPs) > 0 {
		return slices.Map(pod.Status.PodIPs, func(e corev1.PodIP) string {
			return e.IP
		})
	} else if pod.Status.PodIP != "" {
		return []string{pod.Status.PodIP}
	}
	return nil
}
