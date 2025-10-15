package translator

import (
	"fmt"

	envoytypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/maps"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

const (
	// Resource name format strings
	resourceNameFormat = "%s~%s"
	bindKeyFormat      = "%s/%s"
	gatewayNameFormat  = "%s/%s"
)

// AgentGwXdsResources represents XDS resources for a single agent gateway
type AgentGwXdsResources struct {
	types.NamespacedName

	// Status Reports for this gateway
	Reports        reports.ReportMap
	AttachedRoutes map[string]uint

	// Generation of the Gateway object used to produce Reports
	GatewayGeneration int64

	// Resources config for gateway (Bind, Listener, Route)
	ResourceConfig envoycache.Resources

	// Address config (Services, Workloads)
	AddressConfig envoycache.Resources
}

// ResourceName needs to match agentgateway role configured in agentgateway
func (r AgentGwXdsResources) ResourceName() string {
	return fmt.Sprintf(resourceNameFormat, r.Namespace, r.Name)
}

func (r AgentGwXdsResources) Equals(in AgentGwXdsResources) bool {
	return r.NamespacedName == in.NamespacedName &&
		report{reportMap: r.Reports, attachedRoutes: r.AttachedRoutes}.Equals(report{reportMap: in.Reports, attachedRoutes: in.AttachedRoutes}) &&
		r.GatewayGeneration == in.GatewayGeneration &&
		r.ResourceConfig.Version == in.ResourceConfig.Version &&
		r.AddressConfig.Version == in.AddressConfig.Version
}

// AgwResourceWithCustomName is a wrapper type that contains the resource on the gateway, used for the snapshot cache.
type AgwResourceWithCustomName struct {
	proto.Message
	Name    string
	Version uint64
}

func (r AgwResourceWithCustomName) ResourceName() string {
	return r.Name
}

func (r AgwResourceWithCustomName) GetName() string {
	return r.Name
}

func (r AgwResourceWithCustomName) Equals(in AgwResourceWithCustomName) bool {
	return r.Version == in.Version
}

var _ envoytypes.ResourceWithName = AgwResourceWithCustomName{}

type report struct {
	// lower case so krt doesn't error in debug handler
	reportMap      reports.ReportMap
	attachedRoutes map[string]uint
}

// RouteReports contains all route-related Reports
type RouteReports struct {
	HTTPRoutes map[types.NamespacedName]*reports.RouteReport
	GRPCRoutes map[types.NamespacedName]*reports.RouteReport
	TCPRoutes  map[types.NamespacedName]*reports.RouteReport
	TLSRoutes  map[types.NamespacedName]*reports.RouteReport
}

func (r RouteReports) ResourceName() string {
	return "route-reports"
}

func (r RouteReports) Equals(in RouteReports) bool {
	return maps.Equal(r.HTTPRoutes, in.HTTPRoutes) &&
		maps.Equal(r.GRPCRoutes, in.GRPCRoutes) &&
		maps.Equal(r.TCPRoutes, in.TCPRoutes) &&
		maps.Equal(r.TLSRoutes, in.TLSRoutes)
}

// ListenerSetReports contains all listener set Reports
type ListenerSetReports struct {
	Reports map[types.NamespacedName]*reports.ListenerSetReport
}

func (l ListenerSetReports) ResourceName() string {
	return "listenerset-reports"
}

func (l ListenerSetReports) Equals(in ListenerSetReports) bool {
	return maps.Equal(l.Reports, in.Reports)
}

// GatewayReports contains gateway Reports along with attached Routes information
type GatewayReports struct {
	Reports        map[types.NamespacedName]*reports.GatewayReport
	AttachedRoutes map[types.NamespacedName]map[string]uint
	// Generations carries the observed generation per Gateway
	Generations map[types.NamespacedName]int64
}

func (g GatewayReports) ResourceName() string {
	return "gateway-reports"
}

func (g GatewayReports) Equals(in GatewayReports) bool {
	if !maps.Equal(g.Reports, in.Reports) {
		return false
	}

	// Compare AttachedRoutes manually since it contains nested maps
	if len(g.AttachedRoutes) != len(in.AttachedRoutes) {
		return false
	}
	for key, gRoutes := range g.AttachedRoutes {
		inRoutes, exists := in.AttachedRoutes[key]
		if !exists {
			return false
		}
		if !maps.Equal(gRoutes, inRoutes) {
			return false
		}
	}

	// Compare Generations
	if !maps.Equal(g.Generations, in.Generations) {
		return false
	}

	return true
}

func (r report) ResourceName() string {
	return "report"
}

func (r report) Equals(in report) bool {
	if !maps.Equal(r.reportMap.Gateways, in.reportMap.Gateways) {
		return false
	}
	if !maps.Equal(r.reportMap.ListenerSets, in.reportMap.ListenerSets) {
		return false
	}
	if !maps.Equal(r.reportMap.HTTPRoutes, in.reportMap.HTTPRoutes) {
		return false
	}
	if !maps.Equal(r.reportMap.TCPRoutes, in.reportMap.TCPRoutes) {
		return false
	}
	if !maps.Equal(r.reportMap.TLSRoutes, in.reportMap.TLSRoutes) {
		return false
	}
	if !maps.Equal(r.reportMap.Policies, in.reportMap.Policies) {
		return false
	}
	if !maps.Equal(r.attachedRoutes, in.attachedRoutes) {
		return false
	}
	return true
}
