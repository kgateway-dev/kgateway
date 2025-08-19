package plugins

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type AgentgatewayPlugin struct {
	ContributesPolicies map[schema.GroupKind]PolicyPlugin
	// extra has sync beyond primary resources in the collections above
	ExtraHasSynced func() bool
}

func MergePlugins(plug ...AgentgatewayPlugin) AgentgatewayPlugin {
	ret := AgentgatewayPlugin{
		ContributesPolicies: make(map[schema.GroupKind]PolicyPlugin),
	}
	var hasSynced []func() bool
	for _, p := range plug {
		// Merge contributed policies
		for gk, policy := range p.ContributesPolicies {
			ret.ContributesPolicies[gk] = policy
		}
		if p.ExtraHasSynced != nil {
			hasSynced = append(hasSynced, p.ExtraHasSynced)
		}
	}
	ret.ExtraHasSynced = mergeSynced(hasSynced)
	return ret
}

func mergeSynced(funcs []func() bool) func() bool {
	return func() bool {
		for _, f := range funcs {
			if !f() {
				return false
			}
		}
		return true
	}
}

// Plugins registers all built-in policy plugins
func Plugins(agw *AgwCollections) []AgentgatewayPlugin {
	return []AgentgatewayPlugin{
		NewTrafficPlugin(agw),
		NewInferencePlugin(agw),
		NewA2APlugin(agw),
	}
}

func (p AgentgatewayPlugin) HasSynced() bool {
	for _, pol := range p.ContributesPolicies {
		if pol.Policies != nil && !pol.Policies.HasSynced() {
			return false
		}
	}
	if p.ExtraHasSynced != nil && !p.ExtraHasSynced() {
		return false
	}
	return true
}
