package agentgatewaysyncer

import (
	"github.com/agentgateway/agentgateway/go/api"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/slices"

	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
)

func ADPPolicyCollection(binds krt.Collection[ADPResourcesForGateway], agwPlugins plugins.AgentgatewayPlugin) krt.Collection[ADPResourcesForGateway] {
	var allPolicies []krt.Collection[plugins.ADPPolicy]
	// Generate all policies from all registered plugins using contributed policies
	for _, plugin := range agwPlugins.ContributesPolicies {
		allPolicies = append(allPolicies, plugin.ApplyPolicies())
	}
	joinPolicies := krt.JoinCollection(allPolicies, krt.WithName("AllPolicies"))

	// Generate all policies using the plugin system
	allPoliciesCol := krt.NewCollection(binds, func(ctx krt.HandlerContext, i ADPResourcesForGateway) *ADPResourcesForGateway {
		logger.Debug("generating policies for gateway", "gateway", i.Gateway)

		// Convert all plugins.ADPPolicy structs to api.Resource structs
		fetchedPolicies := krt.Fetch(ctx, joinPolicies)
		allResources := slices.Map(fetchedPolicies, func(policy plugins.ADPPolicy) *api.Resource {
			return toADPResource(ADPPolicy{policy.Policy})
		})

		return &ADPResourcesForGateway{
			Resources: allResources,
			Gateway:   i.Gateway,
		}
	})

	return allPoliciesCol
}
