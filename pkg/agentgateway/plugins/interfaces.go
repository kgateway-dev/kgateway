package plugins

import (
	"context"

	"github.com/agentgateway/agentgateway/go/api"
	"istio.io/istio/pilot/pkg/util/protoconv"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/ir"
)

// AgwPolicyStatusSyncHandler defines a function that handles status syncing for a specific policy type in AgentGateway
type AgwPolicyStatusSyncHandler func(ctx context.Context, client client.Client, namespacedName types.NamespacedName, status v1alpha2.PolicyStatus) error

type PolicyPlugin struct {
	Policies       krt.Collection[AgwPolicy]
	PolicyStatuses krt.StatusCollection[controllers.Object, v1alpha2.PolicyStatus]
}

// ApplyPolicies extracts all policies from the collection
func (p *PolicyPlugin) ApplyPolicies() (krt.Collection[AgwPolicy], krt.StatusCollection[controllers.Object, v1alpha2.PolicyStatus]) {
	return p.Policies, p.PolicyStatuses
}

// AgwPolicy wraps an Agw policy for collection handling
type AgwPolicy struct {
	Policy *api.Policy
	// TODO: track errors per policy
}

func (p AgwPolicy) Equals(in AgwPolicy) bool {
	return protoconv.Equals(p.Policy, in.Policy)
}

func (p AgwPolicy) ResourceName() string {
	return p.Policy.Name
}

type AddResourcesPlugin struct {
	Binds     krt.Collection[ir.AgwResourcesForGateway]
	Listeners krt.Collection[ir.AgwResourcesForGateway]
	Routes    krt.Collection[ir.AgwResourcesForGateway]
}

// AddBinds extracts all bind resources from the collection
func (p *AddResourcesPlugin) AddBinds() krt.Collection[ir.AgwResourcesForGateway] {
	return p.Binds
}

// AddListeners extracts all routes resources from the collection
func (p *AddResourcesPlugin) AddListeners() krt.Collection[ir.AgwResourcesForGateway] {
	return p.Listeners
}

// AddRoutes extracts all routes resources from the collection
func (p *AddResourcesPlugin) AddRoutes() krt.Collection[ir.AgwResourcesForGateway] {
	return p.Routes
}
