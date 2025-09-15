package plugins

import (
	"github.com/agentgateway/agentgateway/go/api"
	"istio.io/istio/pilot/pkg/util/protoconv"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/ir"
)

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

type AddResourcesPlugin struct {
	Binds     krt.Collection[ir.ADPResourcesForGateway]
	Listeners krt.Collection[ir.ADPResourcesForGateway]
	Routes    krt.Collection[ir.ADPResourcesForGateway]
}

// AddBinds extracts all bind resources from the collection
func (p *AddResourcesPlugin) AddBinds() krt.Collection[ir.ADPResourcesForGateway] {
	return p.Binds
}

// AddListeners extracts all routes resources from the collection
func (p *AddResourcesPlugin) AddListeners() krt.Collection[ir.ADPResourcesForGateway] {
	return p.Listeners
}

// AddRoutes extracts all routes resources from the collection
func (p *AddResourcesPlugin) AddRoutes() krt.Collection[ir.ADPResourcesForGateway] {
	return p.Routes
}
