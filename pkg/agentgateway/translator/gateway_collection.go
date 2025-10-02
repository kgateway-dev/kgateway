package translator

import (
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

	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// ToResourcep converts a collection of resources to an agentgateway resource group for that gateway
func ToResourcep(gw types.NamespacedName, resources []*api.Resource, rm reports.ReportMap) *ir.AgwResourcesForGateway {
	res := toResource(gw, resources, rm)
	return &res
}

// ToAgwResource converts an internal representation to a resource for agentgateway
func ToAgwResource(t any) *api.Resource {
	switch tt := t.(type) {
	case AgwBind:
		return &api.Resource{Kind: &api.Resource_Bind{Bind: tt.Bind}}
	case AgwListener:
		return &api.Resource{Kind: &api.Resource_Listener{Listener: tt.Listener}}
	case AgwRoute:
		return &api.Resource{Kind: &api.Resource_Route{Route: tt.Route}}
	case AgwTCPRoute:
		return &api.Resource{Kind: &api.Resource_TcpRoute{TcpRoute: tt.TCPRoute}}
	case AgwPolicy:
		return &api.Resource{Kind: &api.Resource_Policy{Policy: tt.Policy}}
	}
	panic("unknown resource kind")
}

// ToResourceWithRoutes converts a collection of resources to an agentgateway resource group for that gateway, along with attached Routes
func ToResourceWithRoutes(gw types.NamespacedName, resources []*api.Resource, attachedRoutes map[string]uint, rm reports.ReportMap) ir.AgwResourcesForGateway {
	return ir.AgwResourcesForGateway{
		Resources:      resources,
		Gateway:        gw,
		Report:         rm,
		AttachedRoutes: attachedRoutes,
	}
}

func toResource(gw types.NamespacedName, resources []*api.Resource, rm reports.ReportMap) ir.AgwResourcesForGateway {
	return ir.AgwResourcesForGateway{
		Resources: resources,
		Gateway:   gw,
		Report:    rm,
	}
}

// AgwBind is a wrapper type that contains the bind on the gateway, as well as the status for the bind.
type AgwBind struct {
	*api.Bind
}

func (g AgwBind) ResourceName() string {
	return g.Key
}

func (g AgwBind) Equals(other AgwBind) bool {
	return protoconv.Equals(g, other)
}

// AgwListener is a wrapper type that contains the listener on the gateway, as well as the status for the listener.
type AgwListener struct {
	*api.Listener
}

func (g AgwListener) ResourceName() string {
	return g.Key
}

func (g AgwListener) Equals(other AgwListener) bool {
	return protoconv.Equals(g, other)
}

// AgwPolicy is a wrapper type that contains the policy on the gateway, as well as the status for the policy.
type AgwPolicy = plugins.AgwPolicy

// AgwBackend is a wrapper type that contains the backend on the gateway, as well as the status for the backend.
type AgwBackend struct {
	*api.Backend
}

func (g AgwBackend) ResourceName() string {
	return g.Name
}

func (g AgwBackend) Equals(other AgwBackend) bool {
	return protoconv.Equals(g, other)
}

// AgwRoute is a wrapper type that contains the route on the gateway, as well as the status for the route.
type AgwRoute struct {
	*api.Route
}

func (g AgwRoute) ResourceName() string {
	return g.Key
}

func (g AgwRoute) Equals(other AgwRoute) bool {
	return protoconv.Equals(g, other)
}

// AgwTCPRoute is a wrapper type that contains the tcp route on the gateway, as well as the status for the tcp route.
type AgwTCPRoute struct {
	*api.TCPRoute
}

func (g AgwTCPRoute) ResourceName() string {
	return g.Key
}

func (g AgwTCPRoute) Equals(other AgwTCPRoute) bool {
	return protoconv.Equals(g, other)
}

// TLSInfo contains the TLS certificate and key for a gateway listener.
type TLSInfo struct {
	Cert []byte
	Key  []byte `json:"-"`
}

// PortBindings is a wrapper type that contains the listener on the gateway, as well as the status for the listener.
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
	Parent     ParentKey
	ParentInfo ParentInfo
	TLSInfo    *TLSInfo
	Valid      bool
	// status for the gateway listener
	Report reports.ReportMap
}

func (g GatewayListener) ResourceName() string {
	return g.Config.Name
}

func (g GatewayListener) Equals(other GatewayListener) bool {
	// TODO: ok to ignore parent/ParentInfo?
	return g.Config.Equals(other.Config) &&
		g.Valid == other.Valid
}

// GatewayCollection returns a collection of the internal representations GatewayListeners for the given gateway.
func GatewayCollection(
	controllerName string,
	gateways krt.Collection[*gwv1.Gateway],
	gatewayClasses krt.Collection[GatewayClass],
	namespaces krt.Collection[*corev1.Namespace],
	grants ReferenceGrants,
	secrets krt.Collection[*corev1.Secret],
	krtopts krtutil.KrtOptions,
) krt.Collection[GatewayListener] {
	gw := krt.NewManyCollection(gateways, func(ctx krt.HandlerContext, obj *gwv1.Gateway) []GatewayListener {
		rm := reports.NewReportMap()
		statusReporter := reports.NewReporter(&rm)
		gwReporter := statusReporter.Gateway(obj)
		logger.Debug("translating Gateway", "gw_name", obj.GetName(), "resource_version", obj.GetResourceVersion())

		class := fetchClass(ctx, gatewayClasses, obj.Spec.GatewayClassName, controllerName)
		if class == nil {
			return nil
		}
		if string(class.Controller) != controllerName {
			return nil // ignore gateways not managed by our controller
		}

		var result []GatewayListener
		kgw := obj.Spec
		status := obj.Status.DeepCopy()
		classControllerName := class.Controller
		var servers []*istio.Server

		// Extract the addresses. A gwv1 will bind to a specific Service
		gatewayServices, err := ExtractGatewayServices(obj)
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
			// Attached Routes count starts at 0 and gets updated later in the status syncer
			// when the real count is available after route processing
			attachedCount := int32(0) // Default to 0 if not found

			server, tlsInfo, programmed := BuildListener(ctx, secrets, grants, namespaces, obj, status, l, i, classControllerName, attachedCount)

			lstatus := status.Listeners[i]

			// Generate supported kinds for the listener
			allowed, _ := GenerateSupportedKinds(l)

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
			meta := ParentMeta(obj, &l.Name)
			// Each listener generates a GatewayListener with a single Server. This allows binding to a specific listener.
			gatewayConfig := Config{
				Meta: Meta{
					CreationTimestamp: obj.CreationTimestamp.Time,
					GroupVersionKind:  schema.GroupVersionKind{Group: wellknown.GatewayGroup, Kind: wellknown.GatewayKind},
					Name:              utils.InternalGatewayName(obj.Namespace, obj.Name, string(l.Name)),
					Annotations:       meta,
					Namespace:         obj.Namespace,
				},
				// TODO: clean up and move away from istio gwv1 ir
				Spec: &istio.Gateway{
					Servers: []*istio.Server{server},
				},
			}
			ref := ParentKey{
				Kind:      wellknown.GatewayGVK,
				Name:      obj.Name,
				Namespace: obj.Namespace,
			}
			pri := ParentInfo{
				InternalName:     utils.InternalGatewayName(obj.Namespace, gatewayConfig.Name, ""),
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
				Parent:     ref,
				ParentInfo: pri,
				Report:     rm,
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

// RouteParents holds information about things Routes can reference as parents.
type RouteParents struct {
	Gateways     krt.Collection[GatewayListener]
	GatewayIndex krt.Index[ParentKey, GatewayListener]
}

// Fetch returns the parents for a given parent key.
func (p RouteParents) Fetch(ctx krt.HandlerContext, pk ParentKey) []*ParentInfo {
	return slices.Map(krt.Fetch(ctx, p.Gateways, krt.FilterIndex(p.GatewayIndex, pk)), func(gw GatewayListener) *ParentInfo {
		return &gw.ParentInfo
	})
}

// BuildRouteParents builds a RouteParents from a collection of gateways.
func BuildRouteParents(
	gateways krt.Collection[GatewayListener],
) RouteParents {
	idx := krt.NewIndex(gateways, "Parent", func(o GatewayListener) []ParentKey {
		return []ParentKey{o.Parent}
	})
	return RouteParents{
		Gateways:     gateways,
		GatewayIndex: idx,
	}
}
