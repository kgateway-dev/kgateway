package agentgatewaysyncer

import (
	"fmt"

	"github.com/agentgateway/agentgateway/go/api"
	istio "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/util/protoconv"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/ptr"
	"istio.io/istio/pkg/slices"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/reports"

	krtinternal "github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

func toResourcep(gw types.NamespacedName, resources []*api.Resource, rm reports.ReportMap) *ADPResourcesForGateway {
	res := toResource(gw, resources, rm)
	return &res
}

func toADPResource(t any) *api.Resource {
	switch tt := t.(type) {
	case ADPBind:
		return &api.Resource{Kind: &api.Resource_Bind{Bind: tt.Bind}}
	case ADPListener:
		return &api.Resource{Kind: &api.Resource_Listener{Listener: tt.Listener}}
	case ADPRoute:
		return &api.Resource{Kind: &api.Resource_Route{Route: tt.Route}}
	case ADPTCPRoute:
		return &api.Resource{Kind: &api.Resource_TcpRoute{TcpRoute: tt.TCPRoute}}
	case ADPPolicy:
		return &api.Resource{Kind: &api.Resource_Policy{Policy: tt.Policy}}
	}
	panic("unknown resource kind")
}

func toResourceWithRoutes(gw types.NamespacedName, resources []*api.Resource, attachedRoutes map[string]uint, rm reports.ReportMap) ADPResourcesForGateway {
	return ADPResourcesForGateway{
		Resources:      resources,
		Gateway:        gw,
		report:         rm,
		attachedRoutes: attachedRoutes,
	}
}

func toResource(gw types.NamespacedName, resources []*api.Resource, rm reports.ReportMap) ADPResourcesForGateway {
	return ADPResourcesForGateway{
		Resources: resources,
		Gateway:   gw,
		report:    rm,
	}
}

type ADPBind struct {
	*api.Bind
}

func (g ADPBind) ResourceName() string {
	return g.Key
}

func (g ADPBind) Equals(other ADPBind) bool {
	return protoconv.Equals(g, other)
}

type ADPListener struct {
	*api.Listener
}

func (g ADPListener) ResourceName() string {
	return g.Key
}

func (g ADPListener) Equals(other ADPListener) bool {
	return protoconv.Equals(g, other)
}

type ADPPolicy struct {
	*api.Policy
}

func (g ADPPolicy) ResourceName() string {
	return "policy/" + g.Name
}

func (g ADPPolicy) Equals(other ADPPolicy) bool {
	return protoconv.Equals(g, other)
}

type ADPBackend struct {
	*api.Backend
}

func (g ADPBackend) ResourceName() string {
	return g.Name
}

func (g ADPBackend) Equals(other ADPBackend) bool {
	return protoconv.Equals(g, other)
}

type ADPRoute struct {
	*api.Route
}

func (g ADPRoute) ResourceName() string {
	return g.Key
}

func (g ADPRoute) Equals(other ADPRoute) bool {
	return protoconv.Equals(g, other)
}

type ADPTCPRoute struct {
	*api.TCPRoute
}

func (g ADPTCPRoute) ResourceName() string {
	return g.Key
}

func (g ADPTCPRoute) Equals(other ADPTCPRoute) bool {
	return protoconv.Equals(g, other)
}

type TLSInfo struct {
	Cert []byte
	Key  []byte `json:"-"`
}

type PortBindings struct {
	GatewayListener
	Port string
}

func (g PortBindings) ResourceName() string {
	return g.GatewayListener.Name
}

func (g PortBindings) Equals(other PortBindings) bool {
	return g.GatewayListener.Equals(other.GatewayListener) &&
		g.Port == other.Port
}

// GatewayListener is a wrapper type that contains the listener on the gateway, as well as the status for the listener.
// This allows binding to a specific listener.
type GatewayListener struct {
	*Config
	parent     parentKey
	parentInfo parentInfo
	TLSInfo    *TLSInfo
	Valid      bool
	// status for the gateway listener
	report reports.ReportMap
}

func (g GatewayListener) ResourceName() string {
	return g.Config.Name
}

func (g GatewayListener) Equals(other GatewayListener) bool {
	// TODO: ok to ignore parent/parentInfo?
	return g.Config.Equals(other.Config) &&
		g.Valid == other.Valid
}

func GatewayCollection(
	agentGatewayClassName string,
	gateways krt.Collection[*gwv1.Gateway],
	gatewayClasses krt.Collection[GatewayClass],
	namespaces krt.Collection[*corev1.Namespace],
	grants ReferenceGrants,
	secrets krt.Collection[*corev1.Secret],
	krtopts krtinternal.KrtOptions,
) krt.Collection[GatewayListener] {
	gw := krt.NewManyCollection(gateways, func(ctx krt.HandlerContext, obj *gwv1.Gateway) []GatewayListener {
		rm := reports.NewReportMap()
		statusReporter := reports.NewReporter(&rm)
		gwReporter := statusReporter.Gateway(obj)
		logger.Debug("translating Gateway", "gw_name", obj.GetName(), "resource_version", obj.GetResourceVersion())

		if string(obj.Spec.GatewayClassName) != agentGatewayClassName {
			return nil // ignore non agentgateway gws
		}

		var result []GatewayListener
		kgw := obj.Spec
		status := obj.Status.DeepCopy()
		class := fetchClass(ctx, gatewayClasses, kgw.GatewayClassName)
		if class == nil {
			return nil
		}
		controllerName := class.Controller
		var servers []*istio.Server

		// Extract the addresses. A gwv1 will bind to a specific Service
		gatewayServices, err := extractGatewayServices(obj)
		if len(gatewayServices) == 0 && err != nil {
			// Short circuit if it's a hard failure
			logger.Error("failed to translate gwv1", "name", obj.GetName(), "namespace", obj.GetNamespace(), "err", err.Message)
			gwReporter.SetCondition(reporter.GatewayCondition{
				Type:    gwv1.GatewayConditionAccepted,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.GatewayReasonInvalid,
				Message: err.Message,
			})
			return nil
		}

		for i, l := range kgw.Listeners {
			server, tlsInfo, programmed := buildListener(ctx, secrets, grants, namespaces, obj, status, l, i, controllerName)
			lstatus := status.Listeners[i]

			// Generate supported kinds for the listener
			allowed, _ := generateSupportedKinds(l)

			// Set all listener conditions from the actual status
			for _, lcond := range lstatus.Conditions {
				gwReporter.Listener(&l).SetCondition(reporter.ListenerCondition{
					Type:    gwv1.ListenerConditionType(lcond.Type),
					Status:  lcond.Status,
					Reason:  gwv1.ListenerConditionReason(lcond.Reason),
					Message: lcond.Message,
				})
			}

			// Set supported kinds for the listener
			gwReporter.Listener(&l).SetSupportedKinds(allowed)

			servers = append(servers, server)
			meta := parentMeta(obj, &l.Name)
			// Each listener generates a GatewayListener with a single Server. This allows binding to a specific listener.
			gatewayConfig := Config{
				Meta: Meta{
					CreationTimestamp: obj.CreationTimestamp.Time,
					GroupVersionKind:  schema.GroupVersionKind{Group: wellknown.GatewayGroup, Kind: wellknown.GatewayKind},
					Name:              InternalGatewayName(obj.Name, string(l.Name)),
					Annotations:       meta,
					Namespace:         obj.Namespace,
				},
				// TODO: clean up and move away from istio gwv1 ir
				Spec: &istio.Gateway{
					Servers: []*istio.Server{server},
				},
			}
			ref := parentKey{
				Kind:      wellknown.GatewayGVK,
				Name:      obj.Name,
				Namespace: obj.Namespace,
			}
			pri := parentInfo{
				InternalName:     obj.Namespace + "/" + gatewayConfig.Name,
				AllowedKinds:     allowed,
				Hostnames:        server.GetHosts(),
				OriginalHostname: string(ptr.OrEmpty(l.Hostname)),
				SectionName:      l.Name,
				Port:             l.Port,
				Protocol:         l.Protocol,
			}

			res := GatewayListener{
				Config:     &gatewayConfig,
				Valid:      programmed,
				TLSInfo:    tlsInfo,
				parent:     ref,
				parentInfo: pri,
				report:     rm,
			}
			gwReporter.SetCondition(reporter.GatewayCondition{
				Type:   gwv1.GatewayConditionAccepted,
				Status: metav1.ConditionTrue,
				Reason: gwv1.GatewayReasonAccepted,
			})
			result = append(result, res)
		}
		return result
	}, krtopts.ToOptions("KubernetesGateway")...)

	return gw
}

// RouteParents holds information about things routes can reference as parents.
type RouteParents struct {
	gateways     krt.Collection[GatewayListener]
	gatewayIndex krt.Index[parentKey, GatewayListener]
}

func (p RouteParents) fetch(ctx krt.HandlerContext, pk parentKey) []*parentInfo {
	return slices.Map(krt.Fetch(ctx, p.gateways, krt.FilterIndex(p.gatewayIndex, pk)), func(gw GatewayListener) *parentInfo {
		return &gw.parentInfo
	})
}

func BuildRouteParents(
	gateways krt.Collection[GatewayListener],
) RouteParents {
	idx := krt.NewIndex(gateways, "parent", func(o GatewayListener) []parentKey {
		return []parentKey{o.parent}
	})
	return RouteParents{
		gateways:     gateways,
		gatewayIndex: idx,
	}
}

// InternalGatewayName returns the name of the internal Istio Gateway corresponding to the
// specified gwv1-api gwv1 and listener.
func InternalGatewayName(gwName, lName string) string {
	return fmt.Sprintf("%s-%s-%s", gwName, AgentgatewayName, lName)
}
