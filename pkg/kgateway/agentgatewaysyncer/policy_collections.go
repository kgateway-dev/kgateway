package agentgatewaysyncer

import (
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

type PolicyStatusCollections = map[schema.GroupKind]krt.StatusCollection[controllers.Object, gwv1.PolicyStatus]

func AgwPolicyCollection(agwPlugins plugins.AgwPlugin, ancestors krt.Collection[*utils.AncestorBackend], krtopts krtutil.KrtOptions) (krt.Collection[ir.AgwResource], PolicyStatusCollections) {
	var allPolicies []krt.Collection[plugins.AgwPolicy]
	policyStatusMap := PolicyStatusCollections{}
	ancestorsIndex := krt.NewIndex(ancestors, "ancestors", func(o *utils.AncestorBackend) []utils.TypedNamespacedName {
		return []utils.TypedNamespacedName{o.Backend}
	})
	ancestorCollection := ancestorsIndex.AsCollection(append(krtopts.ToOptions("AncestorBackend"), utils.TypedNamespacedNameIndexCollectionFunc)...)
	// Collect all policies from registered plugins.
	// Note: Only one plugin should be used per source GVK.
	// Avoid joining collections per-GVK before passing them to a plugin.
	for gvk, plugin := range agwPlugins.ContributesPolicies {
		policy, policyStatus := plugin.ApplyPolicies(plugins.PolicyPluginInput{Ancestors: ancestorCollection})
		allPolicies = append(allPolicies, policy)
		if policyStatus != nil {
			// some plugins may not have a status collection (a2a services, etc.)
			policyStatusMap[gvk] = policyStatus
		}
	}
	joinPolicies := krt.JoinCollection(allPolicies, krtopts.ToOptions("JoinPolicies")...)

	// We add resources for both the policy and the derived backend (if any) to the collection.
	allResourcesCol := krt.NewManyCollection(joinPolicies, func(ctx krt.HandlerContext, i plugins.AgwPolicy) []ir.AgwResource {
		resources := make([]ir.AgwResource, 0, 2)
		resources = append(resources, translator.ToResourceGlobal(i))
		if i.Backend != nil {
			resources = append(resources, translator.ToResourceGlobal(i.Backend))
		}
		return resources
	}, krtopts.ToOptions("AllPoliciesResources")...)

	return allResourcesCol, policyStatusMap
}
