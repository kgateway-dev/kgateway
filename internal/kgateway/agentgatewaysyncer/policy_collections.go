package agentgatewaysyncer

import (
	"github.com/agentgateway/agentgateway/go/api"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/slices"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/translator"
)

func AgwPolicyCollection(binds krt.Collection[ir.AgwResourcesForGateway], agwPlugins plugins.AgwPlugin) (krt.Collection[ir.AgwResourcesForGateway], map[schema.GroupKind]krt.StatusCollection[controllers.Object, v1alpha2.PolicyStatus]) {
	var allPolicies []krt.Collection[plugins.AgwPolicy]
	policyStatusMap := map[schema.GroupKind]krt.StatusCollection[controllers.Object, v1alpha2.PolicyStatus]{}
	// Collect all policies from registered plugins.
	// Note: Only one plugin should be used per source GVK.
	// Avoid joining collections per-GVK before passing them to a plugin.
	for gvk, plugin := range agwPlugins.ContributesPolicies {
		policy, policyStatus := plugin.ApplyPolicies()
		allPolicies = append(allPolicies, policy)
		if policyStatus != nil {
			// some plugins may not have a status collection (a2a services, etc.)
			policyStatusMap[gvk] = policyStatus
		}
	}
	joinPolicies := krt.JoinCollection(allPolicies, krt.WithName("AllPolicies"))

	// Generate all policies using the plugin system
	allPoliciesCol := krt.NewCollection(binds, func(ctx krt.HandlerContext, i ir.AgwResourcesForGateway) *ir.AgwResourcesForGateway {
		logger.Debug("generating policies for gateway", "gateway", i.Gateway)

		// Convert all plugins.AgwPolicy structs to api.Resource structs
		fetchedPolicies := krt.Fetch(ctx, joinPolicies)
		allResources := slices.Map(fetchedPolicies, func(policy plugins.AgwPolicy) *api.Resource {
			return translator.ToAgwResource(translator.AgwPolicy{policy.Policy})
		})

		return &ir.AgwResourcesForGateway{
			Resources: allResources,
			Gateway:   i.Gateway,
		}
	})

	return allPoliciesCol, policyStatusMap
}
