package agentgatewaysyncer

import (
	"net/netip"

	"github.com/agentgateway/agentgateway/go/api"
	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/types"

	krtinternal "github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"

	"istio.io/api/annotation"
	"istio.io/istio/pilot/pkg/features"
	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pilot/pkg/serviceregistry/kube"
	"istio.io/istio/pilot/pkg/util/protoconv"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/config/schema/kind"
	kubeutil "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	kubelabels "istio.io/istio/pkg/kube/labels"
	"istio.io/istio/pkg/log"
	"istio.io/istio/pkg/ptr"
	"istio.io/istio/pkg/slices"
	"istio.io/istio/pkg/util/sets"
)

// index maintains an index of ambient WorkloadInfo objects by various keys.
// These are intentionally pre-computed based on events such that lookups are efficient.
type index struct {
	namespaces krt.Collection[*corev1.Namespace]

	SystemNamespace string
	ClusterID       string
}

// WorkloadsCollection builds out the core Workload object type used in ambient mode.
// A Workload represents a single addressable unit of compute -- typically a Pod or a VM.
// Workloads can come from a variety of sources; these are joined together to build one complete `Collection[WorkloadInfo]`.
func (a *index) WorkloadsCollection(
	pods krt.Collection[*corev1.Pod],
	nodes krt.Collection[Node],
	workloadServices krt.Collection[ServiceInfo],
	endpointSlices krt.Collection[*discovery.EndpointSlice],
	krtopts krtinternal.KrtOptions,
) krt.Collection[WorkloadInfo] {
	WorkloadServicesNamespaceIndex := krt.NewNamespaceIndex(workloadServices)
	EndpointSlicesByIPIndex := endpointSliceAddressIndex(endpointSlices)
	// Workloads coming from pods. There should be one workload for each (running) Pod.
	PodWorkloads := krt.NewCollection(
		pods,
		a.podWorkloadBuilder(
			workloadServices,
			WorkloadServicesNamespaceIndex,
			endpointSlices,
			EndpointSlicesByIPIndex,
			nodes,
		),
		krtopts.ToOptions("PodWorkloads")...,
	)

	// Workloads coming from endpointSlices. These are for *manually added* endpoints. Typically, Kubernetes will insert each pod
	// into the EndpointSlice. This is because Kubernetes has 3 APIs in its model: Service, Pod, and EndpointSlice.
	// In our API, we only have two: Service and Workload.
	// Pod provides much more information than EndpointSlice, so typically we just consume that directly; see method for more details
	// on when we will build from an EndpointSlice.
	EndpointSliceWorkloads := krt.NewManyCollection(
		endpointSlices,
		a.endpointSlicesBuilder(workloadServices),
		krtopts.ToOptions("EndpointSliceWorkloads")...)

	Workloads := krt.JoinCollection(
		[]krt.Collection[WorkloadInfo]{
			PodWorkloads,
			EndpointSliceWorkloads,
		},
		// Each collection has its own unique UID as the key. This guarantees an object can exist in only a single collection
		// This enables us to use the JoinUnchecked optimization.
		append(krtopts.ToOptions("Workloads"), krt.WithJoinUnchecked())...)
	return Workloads
}

func podWorkloadBuilder(
	workloadServices krt.Collection[ServiceInfo],
	workloadServicesNamespaceIndex krt.Index[string, ServiceInfo],
	endpointSlices krt.Collection[*discovery.EndpointSlice],
	endpointSlicesAddressIndex krt.Index[TargetRef, *discovery.EndpointSlice],
	nodes krt.Collection[Node],
	clusterId string,
) krt.TransformationSingle[*corev1.Pod, WorkloadInfo] {
	return func(ctx krt.HandlerContext, p *corev1.Pod) *WorkloadInfo {
		// Pod Is Pending but have a pod IP should be a valid workload, we should build it ,
		// Such as the pod have initContainer which is initialing.
		// See https://github.com/istio/istio/issues/48854
		if kubeutil.CheckPodTerminal(p) {
			return nil
		}
		k8sPodIPs := getPodIPs(p)
		if len(k8sPodIPs) == 0 {
			return nil
		}
		podIPs, err := slices.MapErr(k8sPodIPs, func(e corev1.PodIP) ([]byte, error) {
			n, err := netip.ParseAddr(e.IP)
			if err != nil {
				return nil, err
			}
			return n.AsSlice(), nil
		})
		if err != nil {
			// Is this possible? Probably not in typical case, but anyone could put garbage there.
			return nil
		}

		fo := []krt.FetchOption{krt.FilterIndex(workloadServicesNamespaceIndex, p.Namespace), krt.FilterSelectsNonEmpty(p.GetLabels())}
		if !features.EnableServiceEntrySelectPods {
			fo = append(fo, krt.FilterGeneric(func(a any) bool {
				return a.(ServiceInfo).Source.Kind == kind.Service.String()
			}))
		}
		services := krt.Fetch(ctx, workloadServices, fo...)
		services = append(services, matchingServicesWithoutSelectors(ctx, p, services, workloadServices, endpointSlices, endpointSlicesAddressIndex, kubeutils.GetClusterDomainName())...)
		// Logic from https://github.com/kubernetes/kubernetes/blob/7c873327b679a70337288da62b96dd610858181d/staging/src/k8s.io/endpointslice/utils.go#L37
		// Kubernetes has Ready, Serving, and Terminating. We only have a boolean, which is sufficient for our cases
		status := api.WorkloadStatus_HEALTHY
		if !IsPodReady(p) || p.DeletionTimestamp != nil {
			status = api.WorkloadStatus_UNHEALTHY
		}
		cluster := clusterId

		w := &api.Workload{
			Uid:            generatePodUID(cluster, p),
			Name:           p.Name,
			Namespace:      p.Namespace,
			ClusterId:      clusterId,
			Addresses:      podIPs,
			ServiceAccount: p.Spec.ServiceAccountName,
			Node:           p.Spec.NodeName,
			Services:       constructServices(p, services),
			Status:         status,
			TrustDomain:    pickTrustDomain(),
			Locality:       getPodLocality(ctx, nodes, p),
		}

		if p.Spec.HostNetwork {
			w.NetworkMode = api.NetworkMode_HOST_NETWORK
		}

		w.WorkloadName = workloadName(p)
		w.WorkloadType = api.WorkloadType_POD // backwards compatibility
		w.CanonicalName, w.CanonicalRevision = kubelabels.CanonicalService(p.Labels, w.WorkloadName)

		setTunnelProtocol(p.Labels, p.Annotations, w)
		return precomputeWorkloadPtr(&WorkloadInfo{
			Workload:     w,
			Labels:       p.Labels,
			Source:       kind.Pod,
			CreationTime: p.CreationTimestamp.Time,
		})
	}
}

func (a *index) podWorkloadBuilder(
	workloadServices krt.Collection[ServiceInfo],
	workloadServicesNamespaceIndex krt.Index[string, ServiceInfo],
	endpointSlices krt.Collection[*discovery.EndpointSlice],
	endpointSlicesAddressIndex krt.Index[TargetRef, *discovery.EndpointSlice],
	nodes krt.Collection[Node],
) krt.TransformationSingle[*corev1.Pod, WorkloadInfo] {
	return podWorkloadBuilder(
		workloadServices,
		workloadServicesNamespaceIndex,
		endpointSlices,
		endpointSlicesAddressIndex,
		nodes,
		a.ClusterID,
	)
}

func getPodIPs(p *corev1.Pod) []corev1.PodIP {
	k8sPodIPs := p.Status.PodIPs
	if len(k8sPodIPs) == 0 && p.Status.PodIP != "" {
		k8sPodIPs = []corev1.PodIP{{IP: p.Status.PodIP}}
	}
	return k8sPodIPs
}

// matchingServicesWithoutSelectors finds all Services that match a given pod that do not use selectors.
// See https://kubernetes.io/docs/concepts/services-networking/service/#services-without-selectors for more info.
// For selector service, we query by the selector elsewhere, so this only handles the services that are NOT already found
// by a selector.
// For EndpointSlices that happen to point to the same IP as the pod, but are not directly bound to the pod (via TargetRef),
// we ignore them here. These will produce a model.Workload directly from the EndpointSlice, but with limited information;
// we do not implicitly merge a Pod with an EndpointSlice just based on IP.
func matchingServicesWithoutSelectors(
	ctx krt.HandlerContext,
	p *corev1.Pod,
	alreadyMatchingServices []ServiceInfo,
	workloadServices krt.Collection[ServiceInfo],
	endpointSlices krt.Collection[*discovery.EndpointSlice],
	endpointSlicesAddressIndex krt.Index[TargetRef, *discovery.EndpointSlice],
	domainSuffix string,
) []ServiceInfo {
	var res []ServiceInfo
	// Build out our set of already-matched services to avoid double-selecting a service
	seen := sets.NewWithLength[string](len(alreadyMatchingServices))
	for _, s := range alreadyMatchingServices {
		seen.Insert(s.Service.Hostname)
	}
	tr := TargetRef{
		Kind:      gvk.Pod.Kind,
		Namespace: p.Namespace,
		Name:      p.Name,
		UID:       p.UID,
	}
	// For each IP, find any endpointSlices referencing it.
	matchedSlices := krt.Fetch(ctx, endpointSlices, krt.FilterIndex(endpointSlicesAddressIndex, tr))
	for _, es := range matchedSlices {
		serviceName, f := es.Labels[discovery.LabelServiceName]
		if !f {
			// Not for a service; we don't care about it.
			continue
		}
		hostname := string(kube.ServiceHostname(serviceName, es.Namespace, domainSuffix))
		if seen.Contains(hostname) {
			// We already know about this service
			continue
		}
		// This pod is included in the EndpointSlice. We need to fetch the Service object for it, by key.
		serviceKey := es.Namespace + "/" + hostname
		svcs := krt.Fetch(ctx, workloadServices, krt.FilterKey(serviceKey), krt.FilterGeneric(func(a any) bool {
			// Only find Service, not Service Entry
			return a.(ServiceInfo).Source.Kind == kind.Service.String()
		}))
		if len(svcs) == 0 {
			// no service found
			continue
		}
		// There SHOULD only be one. This is only for `Service` which has unique hostnames.
		svc := svcs[0]
		res = append(res, svc)
	}
	return res
}

func endpointSlicesBuilder(
	workloadServices krt.Collection[ServiceInfo],
	cluster string,
) krt.TransformationMulti[*discovery.EndpointSlice, WorkloadInfo] {
	return func(ctx krt.HandlerContext, es *discovery.EndpointSlice) []WorkloadInfo {
		// EndpointSlices carry port information and a list of IPs.
		// We only care about EndpointSlices that are for a Service.
		// Otherwise, it is just an arbitrary bag of IP addresses for some user-specific purpose, which doesn't have a clear
		// usage for us (if it had some additional info like service account, etc, then perhaps it would be useful).
		serviceName, f := es.Labels[discovery.LabelServiceName]
		if !f {
			return nil
		}
		if es.AddressType == discovery.AddressTypeFQDN {
			// Currently we do not support FQDN. In theory, we could, but its' support in Kubernetes entirely is questionable and
			// may be removed in the near future.
			return nil
		}
		var res []WorkloadInfo
		seen := sets.New[string]()

		// The slice must be for a single service, based on the label above.
		serviceKey := es.Namespace + "/" + string(kube.ServiceHostname(serviceName, es.Namespace, kubeutils.GetClusterDomainName()))
		svcs := krt.Fetch(ctx, workloadServices, krt.FilterKey(serviceKey), krt.FilterGeneric(func(a any) bool {
			// Only find Service, not Service Entry
			return a.(ServiceInfo).Source.Kind == kind.Service.String()
		}))
		if len(svcs) == 0 {
			// no service found
			return nil
		}
		// There SHOULD only be one. This is only Service which has unique hostnames.
		svc := svcs[0]

		// Translate slice ports to our port.
		pl := &api.PortList{Ports: make([]*api.Port, 0, len(es.Ports))}
		for _, p := range es.Ports {
			// We must have name and port (Kubernetes should always set these)
			if p.Name == nil {
				continue
			}
			if p.Port == nil {
				continue
			}
			// We only support TCP for now
			if p.Protocol == nil || *p.Protocol != corev1.ProtocolTCP {
				continue
			}
			// Endpoint slice port has name (service port name, not containerPort) and port (targetPort)
			// We need to join with the Service port list to translate the port name to
			for _, svcPort := range svc.Service.Ports {
				portName := svc.PortNames[int32(svcPort.ServicePort)]
				if portName.PortName != *p.Name {
					continue
				}
				pl.Ports = append(pl.Ports, &api.Port{
					ServicePort: svcPort.ServicePort,
					TargetPort:  uint32(*p.Port),
				})
				break
			}
		}
		services := map[string]*api.PortList{
			serviceKey: pl,
		}

		// Each endpoint in the slice is going to create a Workload
		for _, ep := range es.Endpoints {
			if ep.TargetRef != nil && ep.TargetRef.Kind == gvk.Pod.Kind {
				// Normal case; this is a slice for a pod. We already handle pods, with much more information, so we can skip them
				continue
			}
			// This should not be possible
			if len(ep.Addresses) == 0 {
				continue
			}
			// We currently only support 1 address. Kubernetes will never set more (IPv4 and IPv6 will be two slices), so its mostly undefined.
			key := ep.Addresses[0]
			if seen.InsertContains(key) {
				// Shouldn't happen. Make sure our UID is actually unique
				log.Warnf("IP address %v seen twice in %v/%v", key, es.Namespace, es.Name)
				continue
			}
			health := api.WorkloadStatus_UNHEALTHY
			if ep.Conditions.Ready == nil || *ep.Conditions.Ready {
				health = api.WorkloadStatus_HEALTHY
			}
			// Translate our addresses.
			// Note: users may put arbitrary addresses here. It is recommended by Kubernetes to not
			// give untrusted users EndpointSlice write access.
			addresses, err := slices.MapErr(ep.Addresses, func(e string) ([]byte, error) {
				n, err := netip.ParseAddr(e)
				if err != nil {
					log.Warnf("invalid address in endpointslice %v: %v", e, err)
					return nil, err
				}
				return n.AsSlice(), nil
			})
			if err != nil {
				// If any invalid, skip
				continue
			}
			w := &api.Workload{
				Uid:         cluster + "/discovery.k8s.io/EndpointSlice/" + es.Namespace + "/" + es.Name + "/" + key,
				Name:        es.Name,
				Namespace:   es.Namespace,
				Addresses:   addresses,
				Hostname:    "",
				TrustDomain: pickTrustDomain(),
				Services:    services,
				Status:      health,
				ClusterId:   cluster,
				// For opaque endpoints, we do not know anything about them. They could be overlapping with other IPs, so treat it
				// as a shared address rather than a unique one.
				NetworkMode:           api.NetworkMode_HOST_NETWORK,
				AuthorizationPolicies: nil, // Not support. This can only be used for outbound, so not relevant
				ServiceAccount:        "",  // Unknown. TODO: make this possible to express in ztunnel
				Waypoint:              nil, // Not supported. In theory, we could allow it as an EndpointSlice label, but there is no real use case.
				Locality:              nil, // Not supported. We could maybe, there is a "zone", but it doesn't seem to be well supported
			}
			res = append(res, precomputeWorkload(WorkloadInfo{
				Workload:     w,
				Labels:       nil,
				Source:       kind.EndpointSlice,
				CreationTime: es.CreationTimestamp.Time,
			}))
		}

		return res
	}
}

func (a *index) endpointSlicesBuilder(
	workloadServices krt.Collection[ServiceInfo],
) krt.TransformationMulti[*discovery.EndpointSlice, WorkloadInfo] {
	return endpointSlicesBuilder(
		workloadServices,
		a.ClusterID,
	)
}

func setTunnelProtocol(labels, annotations map[string]string, w *api.Workload) {
	if annotations[annotation.AmbientRedirection.Name] == constants.AmbientRedirectionEnabled {
		// Configured for override
		w.TunnelProtocol = api.TunnelProtocol_HBONE
	}
	// Otherwise supports tunnel directly
	if model.SupportsTunnel(labels, model.TunnelHTTP) {
		w.TunnelProtocol = api.TunnelProtocol_HBONE
		w.NativeTunnel = true
	}
}

func pickTrustDomain() string {
	// TODO: do not hardcode
	return ""
}

func workloadName(pod *corev1.Pod) string {
	objMeta, _ := kubeutil.GetWorkloadMetaFromPod(pod)
	return objMeta.Name
}

func constructServices(p *corev1.Pod, services []ServiceInfo) map[string]*api.PortList {
	res := map[string]*api.PortList{}
	for _, svc := range services {
		n := namespacedHostname(svc.Service.Namespace, svc.Service.Hostname)
		pl := &api.PortList{
			Ports: make([]*api.Port, 0, len(svc.Service.Ports)),
		}
		res[n] = pl
		for _, port := range svc.Service.Ports {
			targetPort := port.TargetPort
			// The svc.Ports represents the api.Service, which drops the port name info and just has numeric target Port.
			// TargetPort can be 0 which indicates its a named port. Check if its a named port and replace with the real targetPort if so.
			if named, f := svc.PortNames[int32(port.ServicePort)]; f && named.TargetPortName != "" {
				// Pods only match on TargetPort names
				tp, ok := FindPortName(p, named.TargetPortName)
				if !ok {
					// Port not present for this workload. Exclude the port entirely
					continue
				}
				targetPort = uint32(tp)
			}

			pl.Ports = append(pl.Ports, &api.Port{
				ServicePort: port.ServicePort,
				TargetPort:  targetPort,
			})
		}
	}
	return res
}

func getPodLocality(ctx krt.HandlerContext, Nodes krt.Collection[Node], pod *corev1.Pod) *api.Locality {
	// NodeName is set by the scheduler after the pod is created
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#late-initialization
	node := krt.FetchOne(ctx, Nodes, krt.FilterKey(pod.Spec.NodeName))
	if node == nil {
		if pod.Spec.NodeName != "" {
			log.Warnf("unable to get node %q for pod %q/%q", pod.Spec.NodeName, pod.Namespace, pod.Name)
		}
		return nil
	}
	return node.Locality
}

// TargetRef is a subset of the Kubernetes ObjectReference which has some fields we don't care about
type TargetRef struct {
	Kind      string
	Namespace string
	Name      string
	UID       types.UID
}

func (t TargetRef) String() string {
	return t.Kind + "/" + t.Namespace + "/" + t.Name + "/" + string(t.UID)
}

// endpointSliceAddressIndex builds an index from IP Address
func endpointSliceAddressIndex(EndpointSlices krt.Collection[*discovery.EndpointSlice]) krt.Index[TargetRef, *discovery.EndpointSlice] {
	return krt.NewIndex(EndpointSlices, "targetRef", func(es *discovery.EndpointSlice) []TargetRef {
		if es.AddressType == discovery.AddressTypeFQDN {
			// Currently we do not support FQDN.
			return nil
		}
		_, f := es.Labels[discovery.LabelServiceName]
		if !f {
			// Not for a service; we don't care about it.
			return nil
		}
		res := make([]TargetRef, 0, len(es.Endpoints))
		for _, ep := range es.Endpoints {
			if ep.TargetRef == nil || ep.TargetRef.Kind != gvk.Pod.Kind {
				// We only want pods here
				continue
			}
			tr := TargetRef{
				Kind:      ep.TargetRef.Kind,
				Namespace: ep.TargetRef.Namespace,
				Name:      ep.TargetRef.Name,
				UID:       ep.TargetRef.UID,
			}
			res = append(res, tr)
		}
		return res
	})
}

func precomputeWorkloadPtr(w *WorkloadInfo) *WorkloadInfo {
	return ptr.Of(precomputeWorkload(*w))
}

func precomputeWorkload(w WorkloadInfo) WorkloadInfo {
	addr := workloadToAddress(w.Workload)
	w.MarshaledAddress = protoconv.MessageToAny(addr)
	w.AsAddress = AddressInfo{
		Address:   addr,
		Marshaled: w.MarshaledAddress,
	}
	return w
}

func workloadToAddress(w *api.Workload) *api.Address {
	return &api.Address{
		Type: &api.Address_Workload{
			Workload: w,
		},
	}
}

// IsPodReady is copied from kubernetes/pkg/api/v1/pod/utils.go
func IsPodReady(pod *corev1.Pod) bool {
	return IsPodReadyConditionTrue(pod.Status)
}

// IsPodReadyConditionTrue returns true if a pod is ready; false otherwise.
func IsPodReadyConditionTrue(status corev1.PodStatus) bool {
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

func generatePodUID(clusterID string, p *corev1.Pod) string {
	return clusterID + "//" + "Pod/" + p.Namespace + "/" + p.Name
}

func FindPortName(pod *corev1.Pod, name string) (int32, bool) {
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			if port.Name == name && port.Protocol == corev1.ProtocolTCP {
				return port.ContainerPort, true
			}
		}
	}
	return 0, false
}

func namespacedHostname(namespace, hostname string) string {
	return namespace + "/" + hostname
}
