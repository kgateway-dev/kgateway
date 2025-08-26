package plugins

import (
	"maps"
	"strings"

	"github.com/agentgateway/agentgateway/go/api"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pilot/pkg/util/protoconv"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

type ADPResourcesForGateway struct {
	// agent gateway dataplane resources
	Resources []*api.Resource
	// gateway name
	Gateway types.NamespacedName
	// status for the gateway
	Report reports.ReportMap
	// track which routes are attached to the gateway listener for each resource type (HTTPRoute, TCPRoute, etc)
	AttachedRoutes map[string]uint
}

func (g ADPResourcesForGateway) ResourceName() string {
	// need a unique name per resource
	return g.Gateway.String() + getResourceListName(g.Resources)
}

func getResourceListName(resources []*api.Resource) string {
	names := make([]string, len(resources))
	for i, res := range resources {
		names[i] = GetADPResourceName(res)
	}
	return strings.Join(names, ",")
}

func GetADPResourceName(r *api.Resource) string {
	switch t := r.GetKind().(type) {
	case *api.Resource_Bind:
		return "bind/" + t.Bind.GetKey()
	case *api.Resource_Listener:
		return "listener/" + t.Listener.GetKey()
	case *api.Resource_Backend:
		return "backend/" + t.Backend.GetName()
	case *api.Resource_Route:
		return "route/" + t.Route.GetKey()
	}
	return "unknown/" + r.String()
}

func (g ADPResourcesForGateway) Equals(other ADPResourcesForGateway) bool {
	// Don't compare reports, as they are not part of the ADPResource equality and synced separately
	for i := range g.Resources {
		if !proto.Equal(g.Resources[i], other.Resources[i]) {
			return false
		}
	}
	if !maps.Equal(g.AttachedRoutes, other.AttachedRoutes) {
		return false
	}
	return g.Gateway == other.Gateway
}

type AddResourcesPlugin struct {
	AdditionalBinds     krt.Collection[ADPResourcesForGateway]
	AdditionalListeners krt.Collection[ADPResourcesForGateway]
	AdditionalWorkloads krt.Collection[ADPResourcesForGateway]
	AdditionalRoutes    krt.Collection[ADPResourcesForGateway]
}

// AddBinds extracts all bind resources from the collection
func (p *AddResourcesPlugin) AddBinds() krt.Collection[ADPResourcesForGateway] {
	return p.AdditionalBinds
}

// AddListeners extracts all routes resources from the collection
func (p *AddResourcesPlugin) AddListeners() krt.Collection[ADPResourcesForGateway] {
	return p.AdditionalListeners
}

// AddWorkloads extracts all workloads resources from the collection
func (p *AddResourcesPlugin) AddWorkloads() krt.Collection[ADPResourcesForGateway] {
	return p.AdditionalWorkloads
}

// AddRoutes extracts all routes resources from the collection
func (p *AddResourcesPlugin) AddRoutes() krt.Collection[ADPResourcesForGateway] {
	return p.AdditionalRoutes
}

type PolicyPlugin struct {
	Policies krt.Collection[ADPPolicy]
}

// ApplyPolicies extracts all policies from the collection
func (p *PolicyPlugin) ApplyPolicies() krt.Collection[ADPPolicy] {
	return p.Policies
}

// ADPPolicy wraps an ADP policy for collection handling
type ADPPolicy struct {
	Policy *api.Policy
	// TODO: track errors per policy
}

func (p ADPPolicy) Equals(in ADPPolicy) bool {
	return protoconv.Equals(p.Policy, in.Policy)
}

func (p ADPPolicy) ResourceName() string {
	return p.Policy.Name + attachmentName(p.Policy.Target)
}

func attachmentName(target *api.PolicyTarget) string {
	if target == nil {
		return ""
	}
	switch v := target.Kind.(type) {
	case *api.PolicyTarget_Gateway:
		return ":" + v.Gateway
	case *api.PolicyTarget_Listener:
		return ":" + v.Listener
	case *api.PolicyTarget_Route:
		return ":" + v.Route
	case *api.PolicyTarget_RouteRule:
		return ":" + v.RouteRule
	case *api.PolicyTarget_Backend:
		return ":" + v.Backend
	default:
		return ""
	}
}
