package agentgatewaysyncer

import (
	"fmt"
	"net/netip"

	"github.com/agentgateway/agentgateway/go/api"
	"istio.io/api/annotation"
	apiv1 "istio.io/api/networking/v1"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pilot/pkg/features"
	"istio.io/istio/pilot/pkg/util/protoconv"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/config/schema/kind"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	kubelabels "istio.io/istio/pkg/kube/labels"
	"istio.io/istio/pkg/log"
	"istio.io/istio/pkg/ptr"
	"istio.io/istio/pkg/slices"
	"istio.io/istio/pkg/util/sets"
	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
)

type NamespaceHostname struct {
	Namespace string
	Hostname  string
}

func (n NamespaceHostname) String() string {
	return n.Namespace + "/" + n.Hostname
}

// index maintains an index of ambient WorkloadInfo objects by various keys.
// These are intentionally pre-computed based on events such that lookups are efficient.
type index struct {
	namespaces krt.Collection[krtcollections.NamespaceMetadata]

	SystemNamespace string
	ClusterID       string
}

// WorkloadsCollection builds out the core Workload object type used in ambient mode.
// A Workload represents a single addressable unit of compute -- typically a Pod or a VM.
// Workloads can come from a variety of sources; these are joined together to build one complete `Collection[WorkloadInfo]`.
func (a *index) WorkloadsCollection(
	pods krt.Collection[krtcollections.WrappedPod],
	workloadServices krt.Collection[ServiceInfo],
	serviceEntries krt.Collection[*networkingclient.ServiceEntry],
	endpointSlices krt.Collection[*discovery.EndpointSlice],
	namespaces krt.Collection[*corev1.Namespace],
	krtopts krtutil.KrtOptions,
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
		),
		krtopts.ToOptions("PodWorkloads")...,
	)
	// TODO(npolshak): Add support for WE?

	// Workloads coming from serviceEntries. These are inlined workloadEntries (under `spec.endpoints`); these serviceEntries will
	// also be generating `api.Service` definitions in the `ServicesCollection` logic.
	//ServiceEntryWorkloads := krt.NewManyCollection(
	//	serviceEntries,
	//	a.serviceEntryWorkloadBuilder(),
	//	krtopts.ToOptions("ServiceEntryWorkloads")...,
	//)

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
			//ServiceEntryWorkloads,
			EndpointSliceWorkloads,
		},
		// Each collection has its own unique UID as the key. This guarantees an object can exist in only a single collection
		// This enables us to use the JoinUnchecked optimization.
		append(krtopts.ToOptions("Workloads"), krt.WithJoinUnchecked())...)
	return Workloads
}

// name format: <cluster>/<group>/<kind>/<namespace>/<name></section-name>
func (a *index) generatePodUID(p krtcollections.WrappedPod) string {
	return a.ClusterID + "//" + "Pod/" + p.Namespace + "/" + p.Name
}

func (a *index) podWorkloadBuilder(
	workloadServices krt.Collection[ServiceInfo],
	workloadServicesNamespaceIndex krt.Index[string, ServiceInfo],
	endpointSlices krt.Collection[*discovery.EndpointSlice],
	endpointSlicesAddressIndex krt.Index[TargetRef, *discovery.EndpointSlice],
) krt.TransformationSingle[krtcollections.WrappedPod, WorkloadInfo] {
	return func(ctx krt.HandlerContext, p krtcollections.WrappedPod) *WorkloadInfo {
		// Pod Is Pending but have a pod IP should be a valid workload, we should build it ,
		// Such as the pod have initContainer which is initialing.
		// See https://github.com/istio/istio/issues/48854
		if p.Terminal {
			return nil
		}
		k8sPodIPs := p.PodIPs
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

		fo := []krt.FetchOption{krt.FilterIndex(workloadServicesNamespaceIndex, p.Namespace), krt.FilterSelectsNonEmpty(p.Labels)}
		if !features.EnableServiceEntrySelectPods {
			fo = append(fo, krt.FilterGeneric(func(a any) bool {
				return a.(ServiceInfo).Source.Kind == kind.Service.String()
			}))
		}
		services := krt.Fetch(ctx, workloadServices, fo...)
		services = append(services, a.matchingServicesWithoutSelectors(ctx, p, services, workloadServices, endpointSlices, endpointSlicesAddressIndex)...)
		// Logic from https://github.com/kubernetes/kubernetes/blob/7c873327b679a70337288da62b96dd610858181d/staging/src/k8s.io/endpointslice/utils.go#L37
		// Kubernetes has Ready, Serving, and Terminating. We only have a boolean, which is sufficient for our cases
		status := api.WorkloadStatus_HEALTHY
		if !p.Ready || p.DeletionTimestamp != nil {
			status = api.WorkloadStatus_UNHEALTHY
		}

		w := &api.Workload{
			Uid:       a.generatePodUID(p),
			Name:      p.Name,
			Namespace: p.Namespace,
			//Network:               network,
			//NetworkGateway:        a.getNetworkGatewayAddress(ctx, network),
			ClusterId:      a.ClusterID,
			Addresses:      podIPs,
			ServiceAccount: p.ServiceAccountName,
			Node:           p.NodeName,
			Services:       constructServices(p, services),
			Status:         status,
			//Locality:              getPodLocality(ctx, nodes, p),
		}

		if p.HostNetwork {
			w.NetworkMode = api.NetworkMode_HOST_NETWORK
		}

		w.WorkloadName = p.WorkloadNameForPod
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

// matchingServicesWithoutSelectors finds all Services that match a given pod that do not use selectors.
// See https://kubernetes.io/docs/concepts/services-networking/service/#services-without-selectors for more info.
// For selector service, we query by the selector elsewhere, so this only handles the services that are NOT already found
// by a selector.
// For EndpointSlices that happen to point to the same IP as the pod, but are not directly bound to the pod (via TargetRef),
// we ignore them here. These will produce a Workload directly from the EndpointSlice, but with limited information;
// we do not implicitly merge a Pod with an EndpointSlice just based on IP.
func (a *index) matchingServicesWithoutSelectors(
	ctx krt.HandlerContext,
	p krtcollections.WrappedPod,
	alreadyMatchingServices []ServiceInfo,
	workloadServices krt.Collection[ServiceInfo],
	endpointSlices krt.Collection[*discovery.EndpointSlice],
	endpointSlicesAddressIndex krt.Index[TargetRef, *discovery.EndpointSlice],
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
		hostname := kubeutils.GetServiceHostname(serviceName, es.Namespace)
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

//func (a *index) serviceEntriesInfo(ctx krt.HandlerContext, s *networkingclient.ServiceEntry) []ServiceInfo {
//	sel := NewSelector(s.Spec.GetWorkloadSelector().GetLabels())
//	portNames := map[int32]ServicePortName{}
//	for _, p := range s.Spec.Ports {
//		portNames[int32(p.Number)] = ServicePortName{
//			PortName: p.Name,
//		}
//	}
//	return slices.Map(a.constructServiceEntries(ctx, s), func(e *api.Service) ServiceInfo {
//		return precomputeService(ServiceInfo{
//			Service:       e,
//			PortNames:     portNames,
//			LabelSelector: sel,
//			Source:        MakeSource(s),
//		})
//	})
//}

// MakeSource is a helper to turn an Object into a model.TypedObject.
func MakeSource(o controllers.Object) TypedObject {
	kind := o.GetObjectKind().GroupVersionKind().Kind
	return TypedObject{
		NamespacedName: config.NamespacedName(o),
		Kind:           kind,
	}
}

func precomputeServicePtr(w *ServiceInfo) *ServiceInfo {
	return ptr.Of(precomputeService(*w))
}

func precomputeService(w ServiceInfo) ServiceInfo {
	addr := serviceToAddress(w.Service)
	w.MarshaledAddress = protoconv.MessageToAny(addr)
	w.AsAddress = AddressInfo{
		Address:   addr,
		Marshaled: w.MarshaledAddress,
	}
	return w
}

func serviceToAddress(s *api.Service) *api.Address {
	return &api.Address{
		Type: &api.Address_Service{
			Service: s,
		},
	}
}

func GetHostAddressesFromServiceEntry(se *networkingclient.ServiceEntry) map[string][]netip.Addr {
	if se == nil {
		return map[string][]netip.Addr{}
	}
	return getHostAddressesFromServiceEntryStatus(&se.Status)
}

func getHostAddressesFromServiceEntryStatus(status *apiv1.ServiceEntryStatus) map[string][]netip.Addr {
	results := map[string][]netip.Addr{}
	for _, addr := range status.GetAddresses() {
		parsed, err := netip.ParseAddr(addr.GetValue())
		if err != nil {
			// strange, we should have written these so it probaby should parse but for now unreadable is unusable and we move on
			continue
		}
		host := addr.GetHost()
		results[host] = append(results[host], parsed)
	}
	return results
}

//func (a *index) constructServiceEntries(ctx krt.HandlerContext, svc *networkingclient.ServiceEntry) []*api.Service {
//	var autoassignedHostAddresses map[string][]netip.Addr
//	addresses, err := slices.MapErr(svc.Spec.Addresses, func(e string) (*api.NetworkAddress, error) {
//		return a.toNetworkAddressFromCidr(ctx, e)
//	})
//	if err != nil {
//		// TODO: perhaps we should support CIDR in the future?
//		return nil
//	}
//	// if this se has autoallocation we can se autoallocated IP, otherwise it will remain an empty slice
//	if ShouldV2AutoAllocateIP(svc) {
//		autoassignedHostAddresses = GetHostAddressesFromServiceEntry(svc)
//	}
//	ports := make([]*api.Port, 0, len(svc.Spec.Ports))
//	for _, p := range svc.Spec.Ports {
//		target := p.TargetPort
//		if target == 0 {
//			target = p.Number
//		}
//		ports = append(ports, &api.Port{
//			ServicePort: p.Number,
//			TargetPort:  target,
//			AppProtocol: toAppProtocolFromIstio(p),
//		})
//	}
//
//	// TODO this is only checking one controller - we may be missing service vips for instances in another cluster
//	res := make([]*api.Service, 0, len(svc.Spec.Hosts))
//	for _, h := range svc.Spec.Hosts {
//		// if we have no user-provided hostsAddresses and h is not wildcarded and we have hostsAddresses supported resolution
//		// we can try to use autoassigned hostsAddresses
//		hostsAddresses := addresses
//		if len(hostsAddresses) == 0 && !host.Name(h).IsWildCarded() && svc.Spec.Resolution != networkingv1beta1.ServiceEntry_NONE {
//			if hostsAddrs, ok := autoassignedHostAddresses[h]; ok {
//				hostsAddresses = slices.Map(hostsAddrs, func(e netip.Addr) *api.NetworkAddress {
//					return a.toNetworkAddressFromIP(ctx, e)
//				})
//			}
//		}
//		res = append(res, &api.Service{
//			Name:            svc.Name,
//			Namespace:       svc.Namespace,
//			Hostname:        h,
//			Addresses:       hostsAddresses,
//			Ports:           ports,
//			SubjectAltNames: svc.Spec.SubjectAltNames,
//			//LoadBalancing:   lb, // TODO: add lb support
//		})
//	}
//	return res
//}
//func ShouldV2AutoAllocateIP(se *networkingclient.ServiceEntry) bool {
//	if se == nil {
//		return false
//	}
//	return shouldV2AutoAllocateIPFromPieces(se.ObjectMeta, &se.Spec)
//}
//
//func shouldV2AutoAllocateIPFromPieces(meta metav1.ObjectMeta, spec *apiv1.ServiceEntry) bool {
//	// if the feature is off we should not assign/use addresses
//	if !features.EnableIPAutoallocate {
//		return false
//	}
//
//	// if resolution is none we cannot honor the assigned IP in the dataplane and should not assign
//	if spec.Resolution == apiv1.ServiceEntry_NONE {
//		return false
//	}
//
//	// check for opt-out by user
//	enabledValue, enabledFound := meta.Labels[label.NetworkingEnableAutoallocateIp.Name]
//	if enabledFound && strings.EqualFold(enabledValue, "false") {
//		return false
//	}
//
//	// if the user assigned their own we don't alloate or use autoassigned addresses
//	if len(spec.Addresses) > 0 {
//		return false
//	}
//
//	return true
//}
//
//// name format: <cluster>/<group>/<kind>/<namespace>/<name></section-name>
//// section name should be the WE address, which needs to be stable across SE updates (it is assumed WE addresses are unique)
//func (a *index) generateServiceEntryUID(svcEntryNamespace, svcEntryName, addr string) string {
//	return a.ClusterID + "/networking.istio.io/ServiceEntry/" + svcEntryNamespace + "/" + svcEntryName + "/" + addr
//}
//func (a *index) serviceEntryWorkloadBuilder() krt.TransformationMulti[*networkingclient.ServiceEntry, WorkloadInfo] {
//	return func(ctx krt.HandlerContext, se *networkingclient.ServiceEntry) []WorkloadInfo {
//		eps := se.Spec.Endpoints
//		// If we have a DNS service, endpoints are not required
//		implicitEndpoints := len(eps) == 0 &&
//			(se.Spec.Resolution == networkingv1alpha3.ServiceEntry_DNS || se.Spec.Resolution == networkingv1alpha3.ServiceEntry_DNS_ROUND_ROBIN) &&
//			se.Spec.WorkloadSelector == nil
//		if len(eps) == 0 && !implicitEndpoints {
//			return nil
//		}
//		// only going to use a subset of the info in `allServices` (since we are building workloads here, not services).
//		allServices := a.serviceEntriesInfo(ctx, se)
//		if implicitEndpoints {
//			eps = slices.Map(allServices, func(si ServiceInfo) *networkingv1alpha3.WorkloadEntry {
//				return &networkingv1alpha3.WorkloadEntry{Address: si.Service.Hostname}
//			})
//		}
//		if len(eps) == 0 {
//			return nil
//		}
//		res := make([]WorkloadInfo, 0, len(eps))
//
//		for i, wle := range eps {
//			services := allServices
//			if implicitEndpoints {
//				// For implicit endpoints, we generate each one from the hostname it was from.
//				// Otherwise, use all.
//				// [i] is safe here since we these are constructed to mirror each other
//				services = []ServiceInfo{allServices[i]}
//			}
//
//			w := &api.Workload{
//				Uid:       a.generateServiceEntryUID(se.Namespace, se.Name, wle.Address),
//				Name:      se.Name,
//				Namespace: se.Namespace,
//				//Network:               network,
//				ClusterId:      a.ClusterID,
//				ServiceAccount: wle.ServiceAccount,
//				Services:       constructServicesFromWorkloadEntry(wle, services),
//				Status:         api.WorkloadStatus_HEALTHY,
//				Locality:       getWorkloadEntryLocality(wle),
//			}
//			if wle.Weight > 0 {
//				w.Capacity = wrappers.UInt32(wle.Weight)
//			}
//
//			if addr, err := netip.ParseAddr(wle.Address); err == nil {
//				w.Addresses = [][]byte{addr.AsSlice()}
//			} else {
//				w.Hostname = wle.Address
//			}
//
//			w.WorkloadName, w.WorkloadType = se.Name, api.WorkloadType_POD // XXX(shashankram): HACK to impersonate pod
//			w.CanonicalName, w.CanonicalRevision = kubelabels.CanonicalService(se.Labels, w.WorkloadName)
//
//			setTunnelProtocol(se.Labels, se.Annotations, w)
//			res = append(res, precomputeWorkload(WorkloadInfo{
//				Workload:     w,
//				Labels:       se.Labels,
//				Source:       kind.WorkloadEntry,
//				CreationTime: se.CreationTimestamp.Time,
//			}))
//		}
//		return res
//	}
//}
//func getWorkloadEntryLocality(p *networkingv1alpha3.WorkloadEntry) *api.Locality {
//	region, zone, subzone := labelutil.SplitLocalityLabel(p.GetLocality())
//	if region == "" && zone == "" && subzone == "" {
//		return nil
//	}
//	return &api.Locality{
//		Region:  region,
//		Zone:    zone,
//		Subzone: subzone,
//	}
//}

func (a *index) endpointSlicesBuilder(
	workloadServices krt.Collection[ServiceInfo],
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
		serviceKey := es.Namespace + "/" + kubeutils.GetServiceHostname(serviceName, es.Namespace)
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
				Uid:       a.ClusterID + "/discovery.k8s.io/EndpointSlice/" + es.Namespace + "/" + es.Name + "/" + key,
				Name:      es.Name,
				Namespace: es.Namespace,
				Addresses: addresses,
				Hostname:  "", // Hostname is never used for Pods, so we set it to empty here
				//Network:     a.Network(ctx).String(),
				Services:  services,
				Status:    health,
				ClusterId: a.ClusterID,
				// For opaque endpoints, we do not know anything about them. They could be overlapping with other IPs, so treat it
				// as a shared address rather than a unique one.
				NetworkMode:           api.NetworkMode_HOST_NETWORK,
				AuthorizationPolicies: nil, // Not support. This can only be used for outbound, so not relevant
				ServiceAccount:        "",  // Unknown.
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

func setTunnelProtocol(labels, annotations map[string]string, w *api.Workload) {
	if annotations[annotation.AmbientRedirection.Name] == constants.AmbientRedirectionEnabled {
		// Configured for override
		w.TunnelProtocol = api.TunnelProtocol_HBONE
	}
	// Otherwise supports tunnel directly
	if SupportsTunnel(labels, TunnelHTTP) {
		w.TunnelProtocol = api.TunnelProtocol_HBONE
		w.NativeTunnel = true
	}
	if w.TunnelProtocol == api.TunnelProtocol_NONE &&
		GetTLSModeFromEndpointLabels(labels) == MutualTLSModeLabel {
		w.TunnelProtocol = api.TunnelProtocol_LEGACY_ISTIO_MTLS
	}
}

//func constructServicesFromWorkloadEntry(p *networkingv1alpha3.WorkloadEntry, services []ServiceInfo) map[string]*api.PortList {
//	res := map[string]*api.PortList{}
//	for _, svc := range services {
//		n := namespacedHostname(svc.Service.Namespace, svc.Service.Hostname)
//		pl := &api.PortList{}
//		res[n] = pl
//		for _, port := range svc.Service.Ports {
//			targetPort := port.TargetPort
//			// Named targetPort has different semantics from Service vs ServiceEntry
//			if svc.Source.Kind == kind.Service.String() {
//				// Service has explicit named targetPorts.
//				if named, f := svc.PortNames[int32(port.ServicePort)]; f && named.TargetPortName != "" {
//					// This port is a named target port, look it up
//					tv, ok := p.Ports[named.TargetPortName]
//					if !ok {
//						// We needed an explicit port, but didn't find one - skip this port
//						continue
//					}
//					targetPort = tv
//				}
//			} else {
//				// ServiceEntry has no explicit named targetPorts; targetPort only allows a number
//				// Instead, there is name matching between the port names
//				if named, f := svc.PortNames[int32(port.ServicePort)]; f {
//					// get port name or target port
//					tv, ok := p.Ports[named.PortName]
//					if ok {
//						// if we match one, override it. Otherwise, use the service port
//						targetPort = tv
//					} else if targetPort == 0 {
//						targetPort = port.ServicePort
//					}
//				}
//			}
//			pl.Ports = append(pl.Ports, &api.Port{
//				ServicePort: port.ServicePort,
//				TargetPort:  targetPort,
//			})
//		}
//	}
//	return res
//}

func constructServices(p krtcollections.WrappedPod, services []ServiceInfo) map[string]*api.PortList {
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
	return krt.NewIndex(EndpointSlices, "endpointslice", func(es *discovery.EndpointSlice) []TargetRef {
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

func (a *index) toNetworkAddress(ctx krt.HandlerContext, vip string) (*api.NetworkAddress, error) {
	ip, err := netip.ParseAddr(vip)
	if err != nil {
		return nil, fmt.Errorf("parse %v: %v", vip, err)
	}
	return &api.NetworkAddress{
		// TODO: calculate network
		Address: ip.AsSlice(),
	}, nil
}

func FindPortName(pod krtcollections.WrappedPod, name string) (int32, bool) {
	for _, ports := range pod.ContainerPorts {
		for _, port := range ports {
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
