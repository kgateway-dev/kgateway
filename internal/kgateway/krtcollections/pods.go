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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// WrappedPod is used by agentgateway as a stripped down representation of the pod
type WrappedPod struct {
	krt.Named
	HostNetwork        bool
	ServiceAccountName string
	NodeName           string
	CreationTimestamp  metav1.Time
	DeletionTimestamp  *metav1.Time
	Labels             map[string]string
	ContainerPorts     map[string][]corev1.ContainerPort
	WorkloadNameForPod string
	UID                types.UID
	Annotations        map[string]string
	// status fields
	Ready    bool
	PodIPs   []corev1.PodIP
	Terminal bool
}

func (c WrappedPod) Equals(in WrappedPod) bool {
	return c.Named == in.Named &&
		c.Ready == in.Ready &&
		c.Terminal == in.Terminal &&
		c.podIPsEquals(c.PodIPs, in.PodIPs) &&
		c.HostNetwork == in.HostNetwork &&
		c.NodeName == in.NodeName &&
		c.ServiceAccountName == in.ServiceAccountName &&
		c.CreationTimestamp.Equal(&in.CreationTimestamp) &&
		(c.DeletionTimestamp == nil && in.DeletionTimestamp == nil ||
			c.DeletionTimestamp != nil && in.DeletionTimestamp != nil &&
				c.DeletionTimestamp.Equal(in.DeletionTimestamp)) &&
		c.mapStringEquals(c.Labels, in.Labels) &&
		c.containerPortsEquals(c.ContainerPorts, in.ContainerPorts) &&
		c.WorkloadNameForPod == in.WorkloadNameForPod &&
		c.UID == in.UID &&
		c.mapStringEquals(c.Annotations, in.Annotations)
}

func (c WrappedPod) podIPsEquals(ips1, ips2 []corev1.PodIP) bool {
	if len(ips1) != len(ips2) {
		return false
	}
	for i, ip1 := range ips1 {
		if ip1.IP != ips2[i].IP {
			return false
		}
	}
	return true
}

func (c WrappedPod) mapStringEquals(m1, m2 map[string]string) bool {
	if len(m1) != len(m2) {
		return false
	}
	for k, v := range m1 {
		if v2, exists := m2[k]; !exists || v != v2 {
			return false
		}
	}
	return true
}

func (c WrappedPod) containerPortsEquals(cp1, cp2 map[string][]corev1.ContainerPort) bool {
	if len(cp1) != len(cp2) {
		return false
	}
	for k, ports1 := range cp1 {
		ports2, exists := cp2[k]
		if !exists || len(ports1) != len(ports2) {
			return false
		}
		for i, port1 := range ports1 {
			port2 := ports2[i]
			if port1.Name != port2.Name ||
				port1.HostPort != port2.HostPort ||
				port1.ContainerPort != port2.ContainerPort ||
				port1.Protocol != port2.Protocol ||
				port1.HostIP != port2.HostIP {
				return false
			}
		}
	}
	return true
}

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

func NewPodsCollection(istioClient kube.Client, krtOptions krtutil.KrtOptions) (krt.Collection[LocalityPod], krt.Collection[WrappedPod]) {
	podClient := kclient.NewFiltered[*corev1.Pod](istioClient, kclient.Filter{
		ObjectTransform: kube.StripPodUnusedFields,
		ObjectFilter:    istioClient.ObjectFilter(),
	})
	pods := krt.WrapClient(podClient, krtOptions.ToOptions("Pods")...)
	nodes := newNodeCollection(istioClient, krtOptions)
	return NewLocalityPodsCollection(nodes, pods, krtOptions), NewPodWrapperCollection(pods, krtOptions)
}

func NewLocalityPodsCollection(nodes krt.Collection[NodeMetadata], pods krt.Collection[*corev1.Pod], krtOptions krtutil.KrtOptions) krt.Collection[LocalityPod] {
	return krt.NewCollection(pods, augmentPodLabels(nodes), krtOptions.ToOptions("AugmentPod")...)
}

func NewPodWrapperCollection(pods krt.Collection[*corev1.Pod], krtOptions krtutil.KrtOptions) krt.Collection[WrappedPod] {
	return krt.NewCollection(pods, func(ctx krt.HandlerContext, obj *corev1.Pod) *WrappedPod {
		objMeta, _ := kube.GetWorkloadMetaFromPod(obj)
		containerPorts := map[string][]corev1.ContainerPort{}
		for _, container := range obj.Spec.Containers {
			containerPorts[container.Name] = []corev1.ContainerPort{}
			for _, port := range container.Ports {
				containerPorts[container.Name] = append(containerPorts[container.Name], port)
			}
		}

		return &WrappedPod{
			Named: krt.Named{
				Name:      obj.Name,
				Namespace: obj.Namespace,
			},
			HostNetwork:        obj.Spec.HostNetwork,
			NodeName:           obj.Spec.NodeName,
			ServiceAccountName: obj.Spec.ServiceAccountName,
			DeletionTimestamp:  obj.GetDeletionTimestamp(),
			CreationTimestamp:  obj.GetCreationTimestamp(),
			Labels:             obj.GetLabels(),
			Annotations:        obj.GetAnnotations(),
			WorkloadNameForPod: objMeta.Name,
			ContainerPorts:     containerPorts,
			UID:                obj.UID,
			// status
			Terminal: checkPodTerminal(obj),
			Ready:    isPodReadyConditionTrue(obj.Status),
			PodIPs:   getPodIPs(obj),
		}
	}, krtOptions.ToOptions("WrappedPod")...)
}

// isPodReadyConditionTrue returns true if a pod is ready; false otherwise.
func isPodReadyConditionTrue(status corev1.PodStatus) bool {
	condition := GetPodReadyCondition(status)
	return condition != nil && condition.Status == corev1.ConditionTrue
}

func GetPodReadyCondition(status corev1.PodStatus) *corev1.PodCondition {
	_, condition := GetPodCondition(&status, corev1.PodReady)
	return condition
}

func GetPodCondition(status *corev1.PodStatus, conditionType corev1.PodConditionType) (int, *corev1.PodCondition) {
	if status == nil {
		return -1, nil
	}
	return GetPodConditionFromList(status.Conditions, conditionType)
}

func checkPodTerminal(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded
}

// GetPodConditionFromList extracts the provided condition from the given list of condition and
// returns the index of the condition and the condition. Returns -1 and nil if the condition is not present.
func GetPodConditionFromList(conditions []corev1.PodCondition, conditionType corev1.PodConditionType) (int, *corev1.PodCondition) {
	if conditions == nil {
		return -1, nil
	}
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return i, &conditions[i]
		}
	}
	return -1, nil
}

func getPodIPs(p *corev1.Pod) []corev1.PodIP {
	k8sPodIPs := p.Status.PodIPs
	if len(k8sPodIPs) == 0 && p.Status.PodIP != "" {
		k8sPodIPs = []corev1.PodIP{{IP: p.Status.PodIP}}
	}
	return k8sPodIPs
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
				labels[corev1.LabelHostname] = nodeName
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
