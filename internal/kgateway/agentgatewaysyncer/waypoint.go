package agentgatewaysyncer

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"

	"github.com/agentgateway/agentgateway/go/api"
	"istio.io/api/annotation"
	"istio.io/api/label"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/log"
	"istio.io/istio/pkg/ptr"
	"istio.io/istio/pkg/slices"
)

type InboundBinding struct {
	Port     uint32
	Protocol api.ApplicationTunnel_Protocol
}

type Waypoint struct {
	krt.Named

	// Addresses this Waypoint is reachable by. For stock Istio waypoints, this
	// is usually the hostname. There will always be at least one address in this
	// list.
	Address *api.GatewayAddress

	// DefaultBinding for an inbound zTunnel to use to connect to a Waypoint it captures.
	// This is applied to the Workloads that are instances of the current Waypoint.
	DefaultBinding *InboundBinding

	// TrafficType controls whether Service or Workload can reference this
	// waypoint. Must be one of "all", "service", "workload".
	TrafficType string

	// ServiceAccounts from instances of the waypoint.
	// This only handles Pods. If we wish to support non-pod waypoints, we'll
	// want to index ServiceEntry/WorkloadEntry or possibly allow specifying
	// the ServiceAccounts directly on a Gateway resource.
	ServiceAccounts []string
	AllowedRoutes   WaypointSelector
}

func (w Waypoint) Equals(other Waypoint) bool {
	return w.Named == other.Named &&
		w.TrafficType == other.TrafficType &&
		ptr.Equal(w.DefaultBinding, other.DefaultBinding) &&
		w.AllowedRoutes.Equals(other.AllowedRoutes) &&
		slices.Equal(w.ServiceAccounts, other.ServiceAccounts) &&
		proto.Equal(w.Address, other.Address)
}

// fetchWaypointForTarget attempts to find the waypoint that should handle traffic for a given service or workload
func fetchWaypointForTarget(
	ctx krt.HandlerContext,
	waypoints krt.Collection[Waypoint],
	namespaces krt.Collection[*corev1.Namespace],
	o metav1.ObjectMeta,
) (*Waypoint, *StatusMessage) {
	// namespace to be used when the annotation doesn't include a namespace
	fallbackNamespace := o.Namespace
	// try fetching the waypoint defined on the object itself
	wp, isNone := getUseWaypoint(o, fallbackNamespace)
	if isNone {
		// we've got a local override here opting out of waypoint
		return nil, nil
	}
	if wp != nil {
		// plausible the object has a waypoint defined but that waypoint's underlying gateway is not ready, in this case we'd return nil here even if
		// the namespace-defined waypoint is ready and would not be nil... is this OK or should we handle that? Could lead to odd behavior when
		// o was reliant on the namespace waypoint and then get's a use-waypoint label added before that gateway is ready.
		// goes from having a waypoint to having no waypoint and then eventually gets a waypoint back
		w := krt.FetchOne[Waypoint](ctx, waypoints, krt.FilterKey(wp.ResourceName()))
		if w != nil {
			if !w.AllowsAttachmentFromNamespaceOrLookup(ctx, namespaces, fallbackNamespace) {
				return nil, ReportWaypointAttachmentDenied(w.ResourceName())
			}
			return w, nil
		}
		// Todo: we may need to pull this from Waypoint, it could be for other reasons
		return nil, ReportWaypointIsNotReady(wp.ResourceName())
	}

	// try fetching the namespace-defined waypoint
	namespace := ptr.OrEmpty[*corev1.Namespace](krt.FetchOne[*corev1.Namespace](ctx, namespaces, krt.FilterKey(o.Namespace)))
	// this probably should never be nil. How would o exist in a namespace we know nothing about? maybe edge case of starting the controller or ns delete?
	if namespace != nil {
		// toss isNone, we don't need to know /why/ we got nil
		wp, _ := getUseWaypoint(namespace.ObjectMeta, fallbackNamespace)
		if wp != nil {
			w := krt.FetchOne[Waypoint](ctx, waypoints, krt.FilterKey(wp.ResourceName()))
			if w != nil {
				if !w.AllowsAttachmentFromNamespace(namespace) {
					return nil, ReportWaypointAttachmentDenied(w.ResourceName())
				}
				return w, nil
			}
			return nil, ReportWaypointIsNotReady(wp.ResourceName())
		}
	}

	// neither o nor it's namespace has a use-waypoint label
	return nil, nil
}

func fetchWaypointForService(ctx krt.HandlerContext, Waypoints krt.Collection[Waypoint],
	Namespaces krt.Collection[*corev1.Namespace], o metav1.ObjectMeta,
) (*Waypoint, *StatusMessage) {
	// This is a waypoint, so it cannot have a waypoint
	if o.Labels[label.GatewayManaged.Name] == constants.ManagedGatewayMeshControllerLabel {
		return nil, nil
	}
	w, err := fetchWaypointForTarget(ctx, Waypoints, Namespaces, o)
	if err != nil || w == nil {
		return nil, err
	}
	if w.TrafficType == constants.ServiceTraffic || w.TrafficType == constants.AllTraffic {
		return w, nil
	}
	// Waypoint does not support Service traffic
	log.Debugf("Unable to add service waypoint %s/%s; traffic type %s not supported for %s/%s",
		w.Namespace, w.Name, w.TrafficType, o.Namespace, o.Name)
	return nil, ReportWaypointUnsupportedTrafficType(w.ResourceName(), constants.ServiceTraffic)
}

// getUseWaypoint takes objectMeta and a defaultNamespace
// it looks for the istio.io/use-waypoint label and parses it
// if there is no namespace provided in the label the default namespace will be used
// defaultNamespace avoids the need to infer when object meta from a namespace was given
func getUseWaypoint(meta metav1.ObjectMeta, defaultNamespace string) (named *krt.Named, isNone bool) {
	if labelValue, ok := meta.Labels[label.IoIstioUseWaypoint.Name]; ok {
		// NOTE: this means Istio reserves the word "none" in this field with a special meaning
		//   a waypoint named "none" cannot be used and will be ignored
		if labelValue == "none" {
			return nil, true
		}
		namespace := defaultNamespace
		if override, f := meta.Labels[label.IoIstioUseWaypointNamespace.Name]; f {
			namespace = override
		}
		return &krt.Named{
			Name:      labelValue,
			Namespace: namespace,
		}, false
	}
	return nil, false
}

func (w Waypoint) ResourceName() string {
	return w.GetNamespace() + "/" + w.GetName()
}

func (a *index) WaypointsCollection(
	gateways krt.Collection[*gatewayv1.Gateway],
	gatewayClasses krt.Collection[*gatewayv1.GatewayClass],
	pods krt.Collection[*corev1.Pod],
	opts krtutil.KrtOptions,
) krt.Collection[Waypoint] {
	podsByNamespace := krt.NewNamespaceIndex(pods)
	return krt.NewCollection(gateways, func(ctx krt.HandlerContext, gateway *gatewayv1.Gateway) *Waypoint {
		if len(gateway.Status.Addresses) == 0 {
			// gateway.Status.Addresses should only be populated once the Waypoint's deployment has at least 1 ready pod, it should never be removed after going ready
			// ignore Kubernetes Gateways which aren't waypoints
			return nil
		}

		instances := krt.Fetch(ctx, pods, krt.FilterLabel(map[string]string{
			label.IoK8sNetworkingGatewayGatewayName.Name: gateway.Name,
		}), krt.FilterIndex(podsByNamespace, gateway.Namespace))

		serviceAccounts := slices.Map(instances, func(p *corev1.Pod) string {
			return p.Spec.ServiceAccountName
		})

		// default traffic type if neither GatewayClass nor Gateway specify a type
		trafficType := constants.ServiceTraffic

		gatewayClass := ptr.OrEmpty(krt.FetchOne(ctx, gatewayClasses, krt.FilterKey(string(gateway.Spec.GatewayClassName))))
		if gatewayClass == nil {
			log.Warnf("could not find GatewayClass %s for Gateway %s/%s", gateway.Spec.GatewayClassName, gateway.Namespace, gateway.Name)
		} else if tt, found := gatewayClass.Labels[label.IoIstioWaypointFor.Name]; found {
			// Check for a declared traffic type that is allowed to pass through the Waypoint's GatewayClass
			trafficType = tt
		}

		// Check for a declared traffic type that is allowed to pass through the Waypoint
		if tt, found := gateway.Labels[label.IoIstioWaypointFor.Name]; found {
			trafficType = tt
		}

		return a.makeWaypoint(ctx, gateway, gatewayClass, serviceAccounts, trafficType)
	}, opts.ToOptions("Waypoints")...)
}

func makeInboundBinding(gateway *gatewayv1.Gateway, gatewayClass *gatewayv1.GatewayClass) *InboundBinding {
	ann, ok := getGatewayOrGatewayClassAnnotation(gateway, gatewayClass)
	if !ok {
		return nil
	}

	// format is either `protocol` or `protocol/port`
	parts := strings.Split(ann, "/")
	if len(parts) == 0 || len(parts) > 2 {
		log.Warnf("invalid value %q for %s. Must be of the format \"<protocol>\" or \"<protocol>/<port>\".", ann, annotation.AmbientWaypointInboundBinding.Name)
		return nil
	}

	// parse protocol
	var protocol api.ApplicationTunnel_Protocol
	switch parts[0] {
	case "NONE":
		protocol = api.ApplicationTunnel_NONE
	case "PROXY":
		protocol = api.ApplicationTunnel_PROXY
	default:
		// Only PROXY is supported for now.
		log.Warnf("invalid protocol %s for %s. Only NONE or PROXY are supported.", parts[0], annotation.AmbientWaypointInboundBinding.Name)
		return nil
	}

	// parse port
	port := uint32(0)
	if len(parts) == 2 {
		parsed, err := strconv.ParseUint(parts[1], 10, 32)
		if err != nil {
			log.Warnf("invalid port %s for %s.", parts[1], annotation.AmbientWaypointInboundBinding.Name)
		}
		port = uint32(parsed)
	}

	return &InboundBinding{
		Port:     port,
		Protocol: protocol,
	}
}

func getGatewayOrGatewayClassAnnotation(gateway *gatewayv1.Gateway, class *gatewayv1.GatewayClass) (string, bool) {
	// Gateway > GatewayClass
	an, ok := gateway.Annotations[annotation.AmbientWaypointInboundBinding.Name]
	if ok {
		return an, true
	}
	if class != nil {
		annotation, ok := class.Annotations[annotation.AmbientWaypointInboundBinding.Name]
		if ok {
			return annotation, true
		}
	}
	return "", false
}

func (a *index) makeWaypoint(
	ctx krt.HandlerContext,
	gateway *gatewayv1.Gateway,
	gatewayClass *gatewayv1.GatewayClass,
	serviceAccounts []string,
	trafficType string,
) *Waypoint {
	binding := makeInboundBinding(gateway, gatewayClass)
	return &Waypoint{
		Named:           krt.NewNamed(gateway),
		Address:         a.getGatewayAddress(ctx, gateway),
		DefaultBinding:  binding,
		AllowedRoutes:   makeAllowedRoutes(gateway, binding),
		TrafficType:     trafficType,
		ServiceAccounts: slices.Sort(serviceAccounts),
	}
}

type WaypointSelector struct {
	FromNamespaces gatewayv1.FromNamespaces
	Selector       labels.Selector
}

func (w WaypointSelector) Equals(other WaypointSelector) bool {
	if w.FromNamespaces != other.FromNamespaces {
		return false
	}
	if (w.Selector) == nil != (other.Selector == nil) {
		return false
	}
	if w.Selector == nil && other.Selector == nil {
		return true
	}
	return w.Selector.String() == other.Selector.String()
}

func (w Waypoint) AllowsAttachmentFromNamespaceOrLookup(ctx krt.HandlerContext, Namespaces krt.Collection[*corev1.Namespace], namespace string) bool {
	switch w.AllowedRoutes.FromNamespaces {
	case gatewayv1.NamespacesFromAll:
		return true
	case gatewayv1.NamespacesFromSelector:
		ns := ptr.OrEmpty[*corev1.Namespace](krt.FetchOne[*corev1.Namespace](ctx, Namespaces, krt.FilterKey(namespace)))
		return w.AllowedRoutes.Selector.Matches(labels.Set(ns.GetLabels()))
	case gatewayv1.NamespacesFromSame:
		return w.Namespace == namespace
	default:
		// Should be impossible
		return w.Namespace == namespace
	}
}

func (w Waypoint) AllowsAttachmentFromNamespace(namespace *corev1.Namespace) bool {
	switch w.AllowedRoutes.FromNamespaces {
	case gatewayv1.NamespacesFromAll:
		return true
	case gatewayv1.NamespacesFromSelector:
		return w.AllowedRoutes.Selector.Matches(labels.Set(namespace.GetLabels()))
	case gatewayv1.NamespacesFromSame:
		return w.Namespace == namespace.Name
	default:
		// Should be impossible
		return w.Namespace == namespace.Name
	}
}

// GetAddress is a nil-safe traversal method for Waypoint
func (w *Waypoint) GetAddress() *api.GatewayAddress {
	if w == nil {
		return nil
	}
	return w.Address
}

// makeAllowedRoutes returns a WaypointSelector that matches the listener with the given binding
// if we don't have a binding we use the default HBONE listener
// if we have a binding we use the protocol and port defined in the binding
func makeAllowedRoutes(gateway *gatewayv1.Gateway, binding *InboundBinding) WaypointSelector {
	// First see if we can find a bound listener
	if listener, found := findBoundListener(gateway, binding); found {
		return makeWaypointSelector(listener)
	}

	// Otherwise use the default HBONE listener
	for _, l := range gateway.Spec.Listeners {
		if l.Protocol == "HBONE" && l.Port == 15008 {
			// This is our HBONE listener
			return makeWaypointSelector(l)
		}
	}

	// We didn't find any listener, just use "Same"
	return WaypointSelector{
		FromNamespaces: gatewayv1.NamespacesFromSame,
	}
}

func findBoundListener(gateway *gatewayv1.Gateway, binding *InboundBinding) (gatewayv1.Listener, bool) {
	if binding == nil {
		return gatewayv1.Listener{}, false
	}
	var match func(l gatewayv1.Listener) bool
	if binding.Port != 0 {
		match = func(l gatewayv1.Listener) bool {
			return l.Port == gatewayv1.PortNumber(binding.Port)
		}
	} else if binding.Protocol == api.ApplicationTunnel_PROXY {
		match = func(l gatewayv1.Listener) bool {
			return l.Protocol == constants.WaypointSandwichListenerProxyProtocol
		}
	}
	for _, l := range gateway.Spec.Listeners {
		if match != nil && match(l) {
			return l, true
		}
	}
	return gatewayv1.Listener{}, false
}

func makeWaypointSelector(l gatewayv1.Listener) WaypointSelector {
	if l.AllowedRoutes == nil || l.AllowedRoutes.Namespaces == nil {
		return WaypointSelector{
			FromNamespaces: gatewayv1.NamespacesFromSame,
		}
	}
	al := *l.AllowedRoutes.Namespaces
	from := ptr.OrDefault(al.From, gatewayv1.NamespacesFromSame)
	label, _ := metav1.LabelSelectorAsSelector(l.AllowedRoutes.Namespaces.Selector)
	return WaypointSelector{
		FromNamespaces: from,
		Selector:       label,
	}
}

func (a *index) getGatewayAddress(ctx krt.HandlerContext, gw *gatewayv1.Gateway) *api.GatewayAddress {
	for _, addr := range gw.Status.Addresses {
		if addr.Type != nil && *addr.Type == gatewayv1.HostnameAddressType {
			// Prefer hostname from status, if we can find it.
			// Hostnames are a more reliable lookup key than IP; hostname is already the unique key for services, and IPs can be re-allocated.
			// Additionally, a destination can have multiple IPs, which makes handling more challenging. For example, was the IPv4 address
			// referenced because we specifically wanted to always use IPv4, or because we happened to pick a random IP among the multiple?
			return &api.GatewayAddress{
				Destination: &api.GatewayAddress_Hostname{
					Hostname: &api.NamespacedHostname{
						Namespace: gw.Namespace,
						Hostname:  addr.Value,
					},
				},
				// TODO: look up the HBONE port instead of hardcoding it
				HboneMtlsPort: 15008,
			}
		}
	}
	// Fallback to IP address
	for _, addr := range gw.Status.Addresses {
		if addr.Type != nil && *addr.Type == gatewayv1.IPAddressType {
			ip, err := netip.ParseAddr(addr.Value)
			if err != nil {
				log.Warnf("parsed invalid IP address %q: %v", addr.Value, err)
				continue
			}
			// Prefer hostname from status, if we can find it.
			return &api.GatewayAddress{
				Destination: &api.GatewayAddress_Address{
					// probably use from Cidr instead?
					Address: a.toNetworkAddressFromIP(ctx, ip),
				},
				// TODO: look up the HBONE port instead of hardcoding it
				HboneMtlsPort: 15008,
			}
		}
	}
	return nil
}

func ReportWaypointIsNotReady(waypoint string) *StatusMessage {
	return &StatusMessage{
		Reason:  "WaypointIsNotReady",
		Message: fmt.Sprintf("waypoint %q is not ready", waypoint),
	}
}

func ReportWaypointAttachmentDenied(waypoint string) *StatusMessage {
	return &StatusMessage{
		Reason:  "AttachmentDenied",
		Message: fmt.Sprintf("we are not permitted to attach to waypoint %q (missing allowedRoutes?)", waypoint),
	}
}

func ReportWaypointUnsupportedTrafficType(waypoint string, ttype string) *StatusMessage {
	return &StatusMessage{
		Reason:  "UnsupportedTrafficType",
		Message: fmt.Sprintf("attempting to bind to traffic type %q which the waypoint %q does not support", ttype, waypoint),
	}
}

func (a *index) toNetworkAddressFromIP(ctx krt.HandlerContext, ip netip.Addr) *api.NetworkAddress {
	return &api.NetworkAddress{
		Network: "", // TODO
		Address: ip.AsSlice(),
	}
}
