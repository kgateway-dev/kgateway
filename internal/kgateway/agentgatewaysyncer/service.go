package agentgatewaysyncer

import (
	"bytes"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/agentgateway/agentgateway/go/api"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	apiannotation "istio.io/api/annotation"
	"istio.io/api/label"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pilot/pkg/features"
	"istio.io/istio/pilot/pkg/serviceregistry/provider"
	"istio.io/istio/pilot/pkg/util/protoconv"
	"istio.io/istio/pkg/cluster"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/config/host"
	kubeutil "istio.io/istio/pkg/config/kube"
	"istio.io/istio/pkg/config/protocol"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/config/schema/kind"
	"istio.io/istio/pkg/config/schema/kubetypes"
	"istio.io/istio/pkg/config/visibility"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/maps"
	"istio.io/istio/pkg/ptr"
	"istio.io/istio/pkg/slices"
	"istio.io/istio/pkg/util/protomarshal"
	"istio.io/istio/pkg/util/sets"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

func (a *index) ServicesCollection(
	services krt.Collection[*corev1.Service],
	serviceEntries krt.Collection[*networkingclient.ServiceEntry],
	waypoints krt.Collection[Waypoint],
	inferencePools krt.Collection[*inf.InferencePool],
	namespaces krt.Collection[*corev1.Namespace],
	krtopts krtutil.KrtOptions,
) krt.Collection[ServiceInfo] {
	servicesInfo := krt.NewCollection(services, a.serviceServiceBuilder(waypoints, namespaces),
		krtopts.ToOptions("ServicesInfo")...)
	//ServiceEntriesInfo := krt.NewManyCollection(serviceEntries, a.serviceEntryServiceBuilder(namespaces),
	//	krtopts.ToOptions("ServiceEntriesInfo")...)
	inferencePoolsInfo := krt.NewCollection(inferencePools, a.inferencePoolBuilder(namespaces),
		krtopts.ToOptions("InferencePools")...)
	//WorkloadServices := krt.JoinCollection([]krt.Collection[ServiceInfo]{ServicesInfo, ServiceEntriesInfo}, krtopts.ToOptions("WorkloadService")...)

	WorkloadServices := krt.JoinCollection([]krt.Collection[ServiceInfo]{servicesInfo, inferencePoolsInfo}, krtopts.ToOptions("WorkloadService")...)
	return WorkloadServices
}

func (a *index) serviceServiceBuilder(
	waypoints krt.Collection[Waypoint],
	namespaces krt.Collection[*corev1.Namespace],
) krt.TransformationSingle[*corev1.Service, ServiceInfo] {
	return func(ctx krt.HandlerContext, s *corev1.Service) *ServiceInfo {
		if s.Spec.Type == corev1.ServiceTypeExternalName {
			// ExternalName services are not implemented by ambient (but will still work).
			// The DNS requests will forward to the upstream DNS server, then Ztunnel can handle the request based on the target
			// hostname.
			// In theory, we could add support for native 'DNS alias' into Ztunnel's DNS proxy. This would give the same behavior
			// but let the DNS proxy handle it instead of forwarding upstream. However, at this time we do not do so.
			return nil
		}
		portNames := map[int32]ServicePortName{}
		for _, p := range s.Spec.Ports {
			portNames[p.Port] = ServicePortName{
				PortName:       p.Name,
				TargetPortName: p.TargetPort.StrVal,
			}
		}
		waypointStatus := WaypointBindingStatus{}
		waypoint, wperr := fetchWaypointForService(ctx, waypoints, namespaces, s.ObjectMeta)
		if waypoint != nil {
			waypointStatus.ResourceName = waypoint.ResourceName()

			// TODO: add this label to the istio api labels so we have constants to use
			if val, ok := s.Labels["istio.io/ingress-use-waypoint"]; ok {
				waypointStatus.IngressLabelPresent = true
				waypointStatus.IngressUseWaypoint = strings.EqualFold(val, "true")
			}
		}
		waypointStatus.Error = wperr

		svc := a.constructService(ctx, s, waypoint)
		return precomputeServicePtr(&ServiceInfo{
			Service:       svc,
			PortNames:     portNames,
			LabelSelector: NewSelector(s.Spec.Selector),
			Source:        MakeSource(s),
			Waypoint:      waypointStatus,
		})
	}
}

// InferenceHostname produces FQDN for a k8s service
func InferenceHostname(name, namespace, domainSuffix string) host.Name {
	return host.Name(name + "." + namespace + "." + "inference" + "." + domainSuffix) // Format: "%s.%s.svc.%s"
}

func (a *index) inferencePoolBuilder(
	namespaces krt.Collection[*corev1.Namespace],
) krt.TransformationSingle[*inf.InferencePool, ServiceInfo] {
	domainSuffix := kubeutils.GetClusterDomainName()
	return func(ctx krt.HandlerContext, s *inf.InferencePool) *ServiceInfo {
		portNames := map[int32]ServicePortName{}
		ports := []*api.Port{{
			ServicePort: uint32(s.Spec.TargetPorts[0].Number), //nolint:gosec // G115: InferencePool TargetPort is int32 with validation 1-65535, always safe
			TargetPort:  uint32(s.Spec.TargetPorts[0].Number), //nolint:gosec // G115: InferencePool TargetPort is int32 with validation 1-65535, always safe
			AppProtocol: api.AppProtocol_HTTP11,
		}}

		// TODO this is only checking one controller - we may be missing service vips for instances in another cluster
		svc := &api.Service{
			Name:      s.Name,
			Namespace: s.Namespace,
			Hostname:  string(InferenceHostname(s.Name, s.Namespace, domainSuffix)),
			Ports:     ports,
		}

		selector := make(map[string]string, len(s.Spec.Selector.MatchLabels))
		for k, v := range s.Spec.Selector.MatchLabels {
			selector[string(k)] = string(v)
		}
		return precomputeServicePtr(&ServiceInfo{
			Service:       svc,
			PortNames:     portNames,
			LabelSelector: NewSelector(selector),
			Source: TypedObject{
				NamespacedName: types.NamespacedName{
					Namespace: s.Namespace,
					Name:      s.Name,
				},
				Kind: "InferencePool", // TODO: get wellknown kind
			},
		})
	}
}

//func (a *index) serviceEntryServiceBuilder(
//	namespaces krt.Collection[*corev1.Namespace],
//) krt.TransformationMulti[*networkingclient.ServiceEntry, ServiceInfo] {
//	return func(ctx krt.HandlerContext, s *networkingclient.ServiceEntry) []ServiceInfo {
//		return a.serviceEntriesInfo(ctx, s)
//	}
//}

func toAppProtocolFromKube(p corev1.ServicePort) api.AppProtocol {
	return toAppProtocolFromProtocol(kubeutil.ConvertProtocol(p.Port, p.Name, p.Protocol, p.AppProtocol))
}

func toAppProtocolFromProtocol(p protocol.Instance) api.AppProtocol {
	switch p {
	case protocol.HTTP:
		return api.AppProtocol_HTTP11
	case protocol.HTTP2:
		return api.AppProtocol_HTTP2
	case protocol.GRPC:
		return api.AppProtocol_GRPC
	}
	return api.AppProtocol_UNKNOWN
}

func (a *index) constructService(ctx krt.HandlerContext, svc *corev1.Service, w *Waypoint) *api.Service {
	ports := make([]*api.Port, 0, len(svc.Spec.Ports))
	for _, p := range svc.Spec.Ports {
		ports = append(ports, &api.Port{
			ServicePort: uint32(p.Port),              //nolint:gosec // G115: Kubernetes service port is int32, always in valid range
			TargetPort:  uint32(p.TargetPort.IntVal), //nolint:gosec // G115: Kubernetes target port is int32, always in valid range
			AppProtocol: toAppProtocolFromKube(p),
		})
	}

	addresses, err := slices.MapErr(getVIPs(svc), func(e string) (*api.NetworkAddress, error) {
		return a.toNetworkAddress(ctx, e)
	})
	if err != nil {
		logger.Warn("fail to parse service", "svc", config.NamespacedName(svc), "error", err)
		return nil
	}

	var lb *api.LoadBalancing

	// The TrafficDistribution field is quite new, so we allow a legacy annotation option as well
	preferClose := strings.EqualFold(svc.Annotations[apiannotation.NetworkingTrafficDistribution.Name], corev1.ServiceTrafficDistributionPreferClose)
	if svc.Spec.TrafficDistribution != nil {
		preferClose = *svc.Spec.TrafficDistribution == corev1.ServiceTrafficDistributionPreferClose
	}
	if preferClose {
		lb = preferCloseLoadBalancer
	}
	if itp := svc.Spec.InternalTrafficPolicy; itp != nil && *itp == corev1.ServiceInternalTrafficPolicyLocal {
		lb = &api.LoadBalancing{
			// Only allow endpoints on the same node.
			RoutingPreference: []api.LoadBalancing_Scope{
				api.LoadBalancing_NODE,
			},
			Mode: api.LoadBalancing_STRICT,
		}
	}
	if svc.Spec.PublishNotReadyAddresses {
		if lb == nil {
			lb = &api.LoadBalancing{}
		}
		lb.HealthPolicy = api.LoadBalancing_ALLOW_ALL
	}

	ipFamily := api.IPFamilies_AUTOMATIC
	if len(svc.Spec.IPFamilies) == 2 {
		ipFamily = api.IPFamilies_DUAL
	} else if len(svc.Spec.IPFamilies) == 1 {
		family := svc.Spec.IPFamilies[0]
		if family == corev1.IPv4Protocol {
			ipFamily = api.IPFamilies_IPV4_ONLY
		} else {
			ipFamily = api.IPFamilies_IPV6_ONLY
		}
	}
	// TODO this is only checking one controller - we may be missing service vips for instances in another cluster
	return &api.Service{
		Name:          svc.Name,
		Namespace:     svc.Namespace,
		Hostname:      kubeutils.ServiceFQDN(svc.ObjectMeta),
		Addresses:     addresses,
		Ports:         ports,
		Waypoint:      w.GetAddress(),
		LoadBalancing: lb,
		IpFamilies:    ipFamily,
	}
}

var preferCloseLoadBalancer = &api.LoadBalancing{
	// Prefer endpoints in close zones, but allow spilling over to further endpoints where required.
	RoutingPreference: []api.LoadBalancing_Scope{
		api.LoadBalancing_NETWORK,
		api.LoadBalancing_REGION,
		api.LoadBalancing_ZONE,
		api.LoadBalancing_SUBZONE,
	},
	Mode: api.LoadBalancing_FAILOVER,
}

func getVIPs(svc *corev1.Service) []string {
	res := []string{}
	cips := svc.Spec.ClusterIPs
	if len(cips) == 0 {
		cips = []string{svc.Spec.ClusterIP}
	}
	for _, cip := range cips {
		if cip != "" && cip != corev1.ClusterIPNone {
			res = append(res, cip)
		}
	}
	return res
}

// Service describes an Istio service (e.g., catalog.mystore.com:8080)
// Each service has a fully qualified domain name (FQDN) and one or more
// ports where the service is listening for connections. *Optionally*, a
// service can have a single load balancer/virtual IP address associated
// with it, such that the DNS queries for the FQDN resolves to the virtual
// IP address (a load balancer IP).
//
// E.g., in kubernetes, a service foo is associated with
// foo.default.svc.cluster.local hostname, has a virtual IP of 10.0.1.1 and
// listens on ports 80, 8080
type Service struct {
	// Attributes contains additional attributes associated with the service
	// used mostly by RBAC for policy enforcement purposes.
	Attributes ServiceAttributes

	// Ports is the set of network ports where the service is listening for
	// connections
	Ports PortList `json:"ports,omitempty"`

	// ServiceAccounts specifies the service accounts that run the service.
	ServiceAccounts []string `json:"serviceAccounts,omitempty"`

	// CreationTime records the time this service was created, if available.
	CreationTime time.Time `json:"creationTime,omitempty"`

	// Name of the service, e.g. "catalog.mystore.com"
	Hostname host.Name `json:"hostname"`

	// ClusterVIPs specifies the service address of the load balancer
	// in each of the clusters where the service resides
	ClusterVIPs AddressMap `json:"clusterVIPs,omitempty"`

	// DefaultAddress specifies the default service IP of the load balancer.
	// Do not access directly. Use GetAddressForProxy
	DefaultAddress string `json:"defaultAddress,omitempty"`

	// AutoAllocatedIPv4Address and AutoAllocatedIPv6Address specifies
	// the automatically allocated IPv4/IPv6 address out of the reserved
	// Class E subnet (240.240.0.0/16) or reserved Benchmarking IP range
	// (2001:2::/48) in RFC5180.for service entries with non-wildcard
	// hostnames. The IPs assigned to services are not
	// synchronized across istiod replicas as the DNS resolution
	// for these service entries happens completely inside a pod
	// whose proxy is managed by one istiod. That said, the algorithm
	// to allocate IPs is pretty deterministic that at stable state, two
	// istiods will allocate the exact same set of IPs for a given set of
	// service entries.
	AutoAllocatedIPv4Address string `json:"autoAllocatedIPv4Address,omitempty"`
	AutoAllocatedIPv6Address string `json:"autoAllocatedIPv6Address,omitempty"`

	// Resolution indicates how the service instances need to be resolved before routing
	// traffic. Most services in the service registry will use static load balancing wherein
	// the proxy will decide the service instance that will receive the traffic. Service entries
	// could either use DNS load balancing (i.e. proxy will query DNS server for the IP of the service)
	// or use the passthrough model (i.e. proxy will forward the traffic to the network endpoint requested
	// by the caller)
	Resolution Resolution

	// ResourceVersion represents the internal version of this object.
	ResourceVersion string
}

func (s *Service) NamespacedName() types.NamespacedName {
	return types.NamespacedName{Name: s.Attributes.Name, Namespace: s.Attributes.Namespace}
}

func (s *Service) Key() string {
	if s == nil {
		return ""
	}

	return s.Attributes.Namespace + "/" + string(s.Hostname)
}

var serviceCmpOpts = []cmp.Option{cmpopts.IgnoreFields(AddressMap{}, "mutex")}

func (s *Service) CmpOpts() []cmp.Option {
	return serviceCmpOpts
}

func (s *Service) SupportsDrainingEndpoints() bool {
	return (features.PersistentSessionLabel != "" && s.Attributes.Labels[features.PersistentSessionLabel] != "") ||
		(features.PersistentSessionHeaderLabel != "" && s.Attributes.Labels[features.PersistentSessionHeaderLabel] != "")
}

// SupportsUnhealthyEndpoints marks if this service should send unhealthy endpoints
func (s *Service) SupportsUnhealthyEndpoints() bool {
	if features.GlobalSendUnhealthyEndpoints.Load() {
		// Enable process-wide
		return true
	}
	if s != nil && s.Attributes.TrafficDistribution != TrafficDistributionAny {
		// When we are doing location aware routing, we need some way to indicate if endpoints are healthy, otherwise we don't
		// know when to spill over to other zones.
		// For the older DestinationRule localityLB, we do this by requiring outlier detection.
		// If they use the newer Kubernetes-native TrafficDistribution we don't want to require an Istio-specific outlier rule,
		// and instead will use endpoint health which requires sending unhealthy endpoints.
		return true
	}
	return false
}

// Resolution indicates how the service instances need to be resolved before routing traffic.
type Resolution int

const (
	// ClientSideLB implies that the proxy will decide the endpoint from its local lb pool
	ClientSideLB Resolution = iota
	// DNSLB implies that the proxy will resolve a DNS address and forward to the resolved address
	DNSLB
	// Passthrough implies that the proxy should forward traffic to the destination IP requested by the caller
	Passthrough
	// DNSRoundRobinLB implies that the proxy will resolve a DNS address and forward to the resolved address
	DNSRoundRobinLB
	// Alias defines a Service that is an alias for another.
	Alias
)

// String converts Resolution in to String.
func (resolution Resolution) String() string {
	switch resolution {
	case ClientSideLB:
		return "ClientSide"
	case DNSLB:
		return "DNS"
	case DNSRoundRobinLB:
		return "DNSRoundRobin"
	case Passthrough:
		return "Passthrough"
	default:
		return fmt.Sprintf("%d", int(resolution))
	}
}

const (
	// TunnelLabel defines the label workloads describe to indicate that they support tunneling.
	// Values are expected to be a CSV list, sorted by preference, of protocols supported.
	// Currently supported values:
	// * "http": indicates tunneling over HTTP over TCP. HTTP/2 vs HTTP/1.1 may be supported by ALPN negotiation.
	// Planned future values:
	// * "http3": indicates tunneling over HTTP over QUIC. This is distinct from "http", since we cannot do ALPN
	//   negotiation for QUIC vs TCP.
	// Users should appropriately parse the full list rather than doing a string literal check to
	// ensure future-proofing against new protocols being added.
	TunnelLabel = "networking.agentgateway.io/tunnel"
	// TunnelLabelShortName is a short name for TunnelLabel to be used in optimized scenarios.
	TunnelLabelShortName = "tunnel"
	// TunnelHTTP indicates tunneling over HTTP over TCP. HTTP/2 vs HTTP/1.1 may be supported by ALPN
	// negotiation. Note: ALPN negotiation is not currently implemented; HTTP/2 will always be used.
	// This is future-proofed, however, because only the `h2` ALPN is exposed.
	TunnelHTTP = "http"
)

const (
	// DisabledTLSModeLabel implies that this endpoint should receive traffic as is (mostly plaintext)
	DisabledTLSModeLabel = "disabled"

	// MutualTLSModeLabel implies that the endpoint is ready to receive agent mTLS connections.
	MutualTLSModeLabel = "istio"
)

func SupportsTunnel(labels map[string]string, tunnelType string) bool {
	tl, f := labels[TunnelLabel]
	if !f {
		return false
	}
	if tl == tunnelType {
		// Fast-path the case where we have only one label
		return true
	}
	// Else check everything. Tunnel label is a comma-separated list.
	return sets.New(strings.Split(tl, ",")...).Contains(tunnelType)
}

// Port represents a network port where a service is listening for
// connections. The port should be annotated with the type of protocol
// used by the port.
type Port struct {
	// Name ascribes a human-readable name for the port object. When a
	// service has multiple ports, the name field is mandatory
	Name string `json:"name,omitempty"`

	// Port number where the service can be reached. Does not necessarily
	// map to the corresponding port numbers for the instances behind the
	// service.
	Port int `json:"port"`

	// Protocol to be used for the port.
	Protocol protocol.Instance `json:"protocol,omitempty"`
}

func (p Port) String() string {
	return fmt.Sprintf("Name:%s Port:%d Protocol:%v", p.Name, p.Port, p.Protocol)
}

// PortList is a set of ports
type PortList []*Port

// ServiceTarget includes a Service object, along with a specific service port
// and target port. This is basically a smaller version of ServiceInstance,
// intended to avoid the need to have the full object when only port information
// is needed.
type ServiceTarget struct {
	Service *Service
	Port    ServiceInstancePort
}

type (
	ServicePort = *Port
	// ServiceInstancePort defines a port that has both a port and targetPort (which distinguishes it from Port)
	// Note: ServiceInstancePort only makes sense in the context of a specific ServiceInstance, because TargetPort depends on a specific instance.
	ServiceInstancePort struct {
		ServicePort
		TargetPort uint32
	}
)

type workloadKind int

const (
	// PodKind indicates the workload is from pod
	PodKind workloadKind = iota
	// WorkloadEntryKind indicates the workload is from workloadentry
	WorkloadEntryKind
)

func (k workloadKind) String() string {
	if k == PodKind {
		return "Pod"
	}

	if k == WorkloadEntryKind {
		return "WorkloadEntry"
	}
	return ""
}

// ServiceAttributes represents a group of custom attributes of the service.
type ServiceAttributes struct {
	// ServiceRegistry indicates the backing service registry system where this service
	// was sourced from.
	// TODO: move the ServiceRegistry type from platform.go to model
	ServiceRegistry provider.ID
	// Name is "destination.service.name" attribute
	Name string
	// Namespace is "destination.service.namespace" attribute
	Namespace string
	// Labels applied to the service
	Labels map[string]string
	// ExportTo defines the visibility of Service in
	// a namespace when the namespace is imported.
	ExportTo sets.Set[visibility.Instance]

	// LabelSelectors are the labels used by the service to select workloads.
	// Applicable to both Kubernetes and ServiceEntries.
	LabelSelectors map[string]string

	// Aliases is the resolved set of aliases for this service. This is computed based on a global view of all Service's `AliasFor`
	// fields.
	// For example, if I had two Services with `externalName: foo`, "a" and "b", then the "foo" service would have Aliases=[a,b].
	Aliases []NamespacedHostname

	// For Kubernetes platform

	// ClusterExternalAddresses is a mapping between a cluster name and the external
	// address(es) to access the service from outside the cluster.
	// Used by the aggregator to aggregate the Attributes.ClusterExternalAddresses
	// for clusters where the service resides
	ClusterExternalAddresses *AddressMap

	// ClusterExternalPorts is a mapping between a cluster name and the service port
	// to node port mappings for a given service. When accessing the service via
	// node port IPs, we need to use the kubernetes assigned node ports of the service
	ClusterExternalPorts map[cluster.ID]map[uint32]uint32

	PassthroughTargetPorts map[uint32]uint32

	K8sAttributes
}

type NamespacedHostname struct {
	Hostname  host.Name
	Namespace string
}

type K8sAttributes struct {
	// Type holds the value of the corev1.Type of the Kubernetes service
	// spec.Type
	Type string

	// spec.ExternalName
	ExternalName string

	// NodeLocal means the proxy will only forward traffic to node local endpoints
	// spec.InternalTrafficPolicy == Local
	NodeLocal bool

	// TrafficDistribution determines the service-level traffic distribution.
	// This may be overridden by locality load balancing settings.
	TrafficDistribution TrafficDistribution

	// ObjectName is the object name of the underlying object. This may differ from the Service.Attributes.Name for legacy semantics.
	ObjectName string

	// spec.PublishNotReadyAddresses
	PublishNotReadyAddresses bool
}

type TrafficDistribution int

const (
	// TrafficDistributionAny allows any destination
	TrafficDistributionAny TrafficDistribution = iota
	// TrafficDistributionPreferClose prefers traffic in same region/zone/network if possible, with failover allowed.
	TrafficDistributionPreferClose TrafficDistribution = iota
)

// DeepCopy creates a deep copy of ServiceAttributes, but skips internal mutexes.
func (s *ServiceAttributes) DeepCopy() ServiceAttributes {
	// AddressMap contains a mutex, which is safe to copy in this case.
	// nolint: govet
	out := *s

	out.Labels = maps.Clone(s.Labels)
	if s.ExportTo != nil {
		out.ExportTo = s.ExportTo.Copy()
	}

	out.LabelSelectors = maps.Clone(s.LabelSelectors)
	out.ClusterExternalAddresses = s.ClusterExternalAddresses.DeepCopy()

	if s.ClusterExternalPorts != nil {
		out.ClusterExternalPorts = make(map[cluster.ID]map[uint32]uint32, len(s.ClusterExternalPorts))
		for k, m := range s.ClusterExternalPorts {
			out.ClusterExternalPorts[k] = maps.Clone(m)
		}
	}

	out.Aliases = slices.Clone(s.Aliases)
	out.PassthroughTargetPorts = maps.Clone(out.PassthroughTargetPorts)

	// AddressMap contains a mutex, which is safe to return a copy in this case.
	// nolint: govet
	return out
}

// Equals checks whether the attributes are equal from the passed in service.
func (s *ServiceAttributes) Equals(other *ServiceAttributes) bool {
	if s == nil {
		return other == nil
	}
	if other == nil {
		return s == nil
	}

	if !maps.Equal(s.Labels, other.Labels) {
		return false
	}

	if !maps.Equal(s.LabelSelectors, other.LabelSelectors) {
		return false
	}

	if !maps.Equal(s.ExportTo, other.ExportTo) {
		return false
	}

	if !slices.Equal(s.Aliases, other.Aliases) {
		return false
	}

	if s.ClusterExternalAddresses.Len() != other.ClusterExternalAddresses.Len() {
		return false
	}

	for k, v1 := range s.ClusterExternalAddresses.GetAddresses() {
		if v2, ok := other.ClusterExternalAddresses.Addresses[k]; !ok || !slices.Equal(v1, v2) {
			return false
		}
	}

	if len(s.ClusterExternalPorts) != len(other.ClusterExternalPorts) {
		return false
	}

	for k, v1 := range s.ClusterExternalPorts {
		if v2, ok := s.ClusterExternalPorts[k]; !ok || !maps.Equal(v1, v2) {
			return false
		}
	}
	return s.Name == other.Name && s.Namespace == other.Namespace &&
		s.ServiceRegistry == other.ServiceRegistry && s.K8sAttributes == other.K8sAttributes
}

type AddressInfo struct {
	*api.Address
	Marshaled *anypb.Any
}

func (i AddressInfo) Equals(other AddressInfo) bool {
	return protoconv.Equals(i.Address, other.Address)
}

func (i AddressInfo) Aliases() []string {
	switch addr := i.Type.(type) {
	case *api.Address_Workload:
		aliases := make([]string, 0, len(addr.Workload.GetAddresses()))
		network := addr.Workload.GetNetwork()
		for _, workloadAddr := range addr.Workload.GetAddresses() {
			ip, _ := netip.AddrFromSlice(workloadAddr)
			aliases = append(aliases, network+"/"+ip.String())
		}
		return aliases
	case *api.Address_Service:
		aliases := make([]string, 0, len(addr.Service.GetAddresses()))
		for _, networkAddr := range addr.Service.GetAddresses() {
			ip, _ := netip.AddrFromSlice(networkAddr.GetAddress())
			aliases = append(aliases, networkAddr.GetNetwork()+"/"+ip.String())
		}
		return aliases
	}
	return nil
}

func (i AddressInfo) ResourceName() string {
	var name string
	switch addr := i.Type.(type) {
	case *api.Address_Workload:
		name = workloadResourceName(addr.Workload)
	case *api.Address_Service:
		name = serviceResourceName(addr.Service)
	}
	return name
}

type TypedObject struct {
	types.NamespacedName
	Kind string
}

type ServicePortName struct {
	PortName       string
	TargetPortName string
}

type ServiceInfo struct {
	Service *api.Service
	// LabelSelectors for the Service. Note these are only used internally, not sent over XDS
	LabelSelector LabelSelector
	// PortNames provides a mapping of ServicePort -> port names. Note these are only used internally, not sent over XDS
	PortNames map[int32]ServicePortName
	// Source is the type that introduced this service.
	Source   TypedObject
	Waypoint WaypointBindingStatus
	// MarshaledAddress contains the pre-marshaled representation.
	// Note: this is an Address -- not a Service.
	MarshaledAddress *anypb.Any
	// AsAddress contains a pre-created AddressInfo representation. This ensures we do not need repeated conversions on
	// the hotpath
	AsAddress AddressInfo
}

func (i ServiceInfo) GetLabelSelector() map[string]string {
	return i.LabelSelector.Labels
}

func (i ServiceInfo) GetStatusTarget() TypedObject {
	return i.Source
}

type StatusMessage struct {
	Reason  string
	Message string
}

type WaypointBindingStatus struct {
	// ResourceName that clients should use when addressing traffic to this Service.
	ResourceName string
	// IngressUseWaypoint specifies whether ingress gateways should use the waypoint for this service.
	IngressUseWaypoint bool
	// IngressLabelPresent specifies whether the istio.io/ingress-use-waypoint label is set on the service.
	IngressLabelPresent bool
	// Error represents some error
	Error *StatusMessage
}

func (i WaypointBindingStatus) Equals(other WaypointBindingStatus) bool {
	return i.ResourceName == other.ResourceName &&
		i.IngressUseWaypoint == other.IngressUseWaypoint &&
		i.IngressLabelPresent == other.IngressLabelPresent &&
		ptr.Equal(i.Error, other.Error)
}

func (i ServiceInfo) NamespacedName() types.NamespacedName {
	return types.NamespacedName{Name: i.Service.GetName(), Namespace: i.Service.GetNamespace()}
}

func (i ServiceInfo) GetNamespace() string {
	return i.Service.GetNamespace()
}

func (i ServiceInfo) Equals(other ServiceInfo) bool {
	return equalUsingPremarshaled(i.Service, i.MarshaledAddress, other.Service, other.MarshaledAddress) &&
		maps.Equal(i.LabelSelector.Labels, other.LabelSelector.Labels) &&
		maps.Equal(i.PortNames, other.PortNames) &&
		i.Source == other.Source
}

func (i ServiceInfo) ResourceName() string {
	return serviceResourceName(i.Service)
}

func serviceResourceName(s *api.Service) string {
	// TODO: check prepending svc
	return s.GetNamespace() + "/" + s.GetHostname()
}

type WorkloadInfo struct {
	Workload *api.Workload
	// Labels for the workload. Note these are only used internally, not sent over XDS
	Labels map[string]string
	// Source is the type that introduced this workload.
	Source kind.Kind
	// CreationTime is the time when the workload was created. Note this is used internally only.
	CreationTime time.Time
	// MarshaledAddress contains the pre-marshaled representation.
	// Note: this is an Address -- not a Workload.
	MarshaledAddress *anypb.Any
	// AsAddress contains a pre-created AddressInfo representation. This ensures we do not need repeated conversions on
	// the hotpath
	AsAddress AddressInfo
}

func (i WorkloadInfo) Equals(other WorkloadInfo) bool {
	return equalUsingPremarshaled(i.Workload, i.MarshaledAddress, other.Workload, other.MarshaledAddress) &&
		maps.Equal(i.Labels, other.Labels) &&
		i.Source == other.Source &&
		i.CreationTime == other.CreationTime
}

func workloadResourceName(w *api.Workload) string {
	return w.GetUid()
}

func (i *WorkloadInfo) Clone() *WorkloadInfo {
	return &WorkloadInfo{
		Workload:     protomarshal.Clone(i.Workload),
		Labels:       maps.Clone(i.Labels),
		Source:       i.Source,
		CreationTime: i.CreationTime,
	}
}

func (i WorkloadInfo) ResourceName() string {
	return workloadResourceName(i.Workload)
}

type LabelSelector struct {
	Labels map[string]string
}

func NewSelector(l map[string]string) LabelSelector {
	return LabelSelector{l}
}

func (l LabelSelector) GetLabelSelector() map[string]string {
	return l.Labels
}

// MCSServiceInfo combines the name of a service with a particular Kubernetes cluster. This
// is used for debug information regarding the state of Kubernetes Multi-Cluster Services (MCS).
type MCSServiceInfo struct {
	Cluster         cluster.ID
	Name            string
	Namespace       string
	Exported        bool
	Imported        bool
	ClusterSetVIP   string
	Discoverability map[host.Name]string
}

// GetNames returns port names
func (ports PortList) GetNames() []string {
	names := make([]string, 0, len(ports))
	for _, port := range ports {
		names = append(names, port.Name)
	}
	return names
}

// Get retrieves a port declaration by name
func (ports PortList) Get(name string) (*Port, bool) {
	for _, port := range ports {
		if port.Name == name {
			return port, true
		}
	}
	return nil, false
}

// GetByPort retrieves a port declaration by port value
func (ports PortList) GetByPort(num int) (*Port, bool) {
	for _, port := range ports {
		if port.Port == num && port.Protocol != protocol.UDP {
			return port, true
		}
	}
	return nil, false
}

func (p *Port) Equals(other *Port) bool {
	if p == nil {
		return other == nil
	}
	if other == nil {
		return p == nil
	}
	return p.Name == other.Name && p.Port == other.Port && p.Protocol == other.Protocol
}

func (ports PortList) Equals(other PortList) bool {
	return slices.EqualFunc(ports, other, func(a, b *Port) bool {
		return a.Equals(b)
	})
}

func (ports PortList) String() string {
	sp := make([]string, 0, len(ports))
	for _, p := range ports {
		sp = append(sp, p.String())
	}
	return strings.Join(sp, ", ")
}

// HasAddressOrAssigned returns whether the service has an IP address.
// This includes auto-allocated IP addresses. Note that not all proxies support auto-allocated IP addresses;
// typically GetAllAddressesForProxy should be used which automatically filters addresses to account for that.
func (s *Service) HasAddressOrAssigned(id cluster.ID) bool {
	if id != "" {
		if len(s.ClusterVIPs.GetAddressesFor(id)) > 0 {
			return true
		}
	}
	if s.DefaultAddress != constants.UnspecifiedIP {
		return true
	}
	if s.AutoAllocatedIPv4Address != "" {
		return true
	}
	if s.AutoAllocatedIPv6Address != "" {
		return true
	}
	return false
}

// GetTLSModeFromEndpointLabels returns the value of the label
// security.istio.io/tlsMode if set. Do not return Enums or constants
// from this function as users could provide values other than istio/disabled
// and apply custom transport socket matchers here.
func GetTLSModeFromEndpointLabels(labels map[string]string) string {
	if labels != nil {
		if val, exists := labels[label.SecurityTlsMode.Name]; exists {
			return val
		}
	}
	return DisabledTLSModeLabel
}

// DeepCopy creates a clone of Service.
func (s *Service) DeepCopy() *Service {
	// nolint: govet
	out := *s
	out.Attributes = s.Attributes.DeepCopy()
	if s.Ports != nil {
		out.Ports = make(PortList, len(s.Ports))
		for i, port := range s.Ports {
			if port != nil {
				out.Ports[i] = &Port{
					Name:     port.Name,
					Port:     port.Port,
					Protocol: port.Protocol,
				}
			} else {
				out.Ports[i] = nil
			}
		}
	}

	out.ServiceAccounts = slices.Clone(s.ServiceAccounts)
	out.ClusterVIPs = *s.ClusterVIPs.DeepCopy()
	return &out
}

// Equals compares two service objects.
func (s *Service) Equals(other *Service) bool {
	if s == nil {
		return other == nil
	}
	if other == nil {
		return s == nil
	}

	if !s.Attributes.Equals(&other.Attributes) {
		return false
	}

	if !s.Ports.Equals(other.Ports) {
		return false
	}
	if !slices.Equal(s.ServiceAccounts, other.ServiceAccounts) {
		return false
	}

	if len(s.ClusterVIPs.Addresses) != len(other.ClusterVIPs.Addresses) {
		return false
	}
	for k, v1 := range s.ClusterVIPs.Addresses {
		if v2, ok := other.ClusterVIPs.Addresses[k]; !ok || !slices.Equal(v1, v2) {
			return false
		}
	}

	return s.DefaultAddress == other.DefaultAddress && s.AutoAllocatedIPv4Address == other.AutoAllocatedIPv4Address &&
		s.AutoAllocatedIPv6Address == other.AutoAllocatedIPv6Address && s.Hostname == other.Hostname &&
		s.Resolution == other.Resolution
}

func equalUsingPremarshaled[T proto.Message](a T, am *anypb.Any, b T, bm *anypb.Any) bool {
	// If they are both pre-marshaled, use the marshaled representation. This is orders of magnitude faster
	if am != nil && bm != nil {
		return bytes.Equal(am.GetValue(), bm.GetValue())
	}

	// Fallback to equals
	return protoconv.Equals(a, b)
}

// AddressMap provides a thread-safe mapping of addresses for each Kubernetes cluster.
type AddressMap struct {
	// Addresses hold the underlying map. Most code should only access this through the available methods.
	// Should only be used by tests and construction/initialization logic, where there is no concern
	// for race conditions.
	Addresses map[cluster.ID][]string

	// NOTE: The copystructure library is not able to copy unexported fields, so the mutex will not be copied.
	mutex sync.RWMutex
}

func (m *AddressMap) Len() int {
	if m == nil {
		return 0
	}
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return len(m.Addresses)
}

func (m *AddressMap) DeepCopy() *AddressMap {
	if m == nil {
		return nil
	}
	return &AddressMap{
		Addresses: m.GetAddresses(),
	}
}

// GetAddresses returns the mapping of clusters to addresses.
func (m *AddressMap) GetAddresses() map[cluster.ID][]string {
	if m == nil {
		return nil
	}

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.Addresses == nil {
		return nil
	}

	out := make(map[cluster.ID][]string)
	for k, v := range m.Addresses {
		out[k] = slices.Clone(v)
	}
	return out
}

// SetAddresses sets the addresses per cluster.
func (m *AddressMap) SetAddresses(addrs map[cluster.ID][]string) {
	if len(addrs) == 0 {
		addrs = nil
	}

	m.mutex.Lock()
	m.Addresses = addrs
	m.mutex.Unlock()
}

func (m *AddressMap) GetAddressesFor(c cluster.ID) []string {
	if m == nil {
		return nil
	}

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.Addresses == nil {
		return nil
	}

	// Copy the Addresses array.
	return append([]string{}, m.Addresses[c]...)
}

func (m *AddressMap) SetAddressesFor(c cluster.ID, addresses []string) *AddressMap {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if len(addresses) == 0 {
		// Setting an empty array for the cluster. Remove the entry for the cluster if it exists.
		if m.Addresses != nil {
			delete(m.Addresses, c)

			// Delete the map if there's nothing left.
			if len(m.Addresses) == 0 {
				m.Addresses = nil
			}
		}
	} else {
		// Create the map if it doesn't already exist.
		if m.Addresses == nil {
			m.Addresses = make(map[cluster.ID][]string)
		}
		m.Addresses[c] = addresses
	}
	return m
}

func (m *AddressMap) AddAddressesFor(c cluster.ID, addresses []string) *AddressMap {
	if len(addresses) == 0 {
		return m
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Create the map if nil.
	if m.Addresses == nil {
		m.Addresses = make(map[cluster.ID][]string)
	}

	m.Addresses[c] = append(m.Addresses[c], addresses...)
	return m
}

func (m *AddressMap) ForEach(fn func(c cluster.ID, addresses []string)) {
	if m == nil {
		return
	}

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.Addresses == nil {
		return
	}

	for c, addresses := range m.Addresses {
		fn(c, addresses)
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

// MakeSource is a helper to turn an Object into a model.TypedObject.
func MakeSource(o controllers.Object) TypedObject {
	kind := gvk.MustToKind(kubetypes.GvkFromObject(o)).String()
	return TypedObject{
		NamespacedName: config.NamespacedName(o),
		Kind:           kind,
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
