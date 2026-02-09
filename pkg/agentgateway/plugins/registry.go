package plugins

import (
	"maps"

	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/agentgateway"
)

type AgwPlugin struct {
	AddResourceExtension *AddResourcesPlugin
	ContributesPolicies  map[schema.GroupKind]PolicyPlugin
	// DerivedBackends contains backends derived from policies (e.g., static backends from URIs)
	DerivedBackends krt.Collection[*agentgateway.AgentgatewayBackend]
}

func MergePlugins(plug ...AgwPlugin) AgwPlugin {
	ret := AgwPlugin{
		ContributesPolicies: make(map[schema.GroupKind]PolicyPlugin),
	}
	var derivedBackends []krt.Collection[*agentgateway.AgentgatewayBackend]
	for _, p := range plug {
		// Merge contributed policies
		maps.Copy(ret.ContributesPolicies, p.ContributesPolicies)
		if p.AddResourceExtension != nil {
			if ret.AddResourceExtension == nil {
				ret.AddResourceExtension = &AddResourcesPlugin{}
			}
			if ret.AddResourceExtension.Binds == nil {
				ret.AddResourceExtension.Binds = p.AddResourceExtension.Binds
			}
			if p.AddResourceExtension.Listeners != nil {
				ret.AddResourceExtension.Listeners = p.AddResourceExtension.Listeners
			}
			if p.AddResourceExtension.Routes != nil {
				ret.AddResourceExtension.Routes = p.AddResourceExtension.Routes
			}
		}
		if p.DerivedBackends != nil {
			derivedBackends = append(derivedBackends, p.DerivedBackends)
		}
	}
	if len(derivedBackends) > 0 {
		ret.DerivedBackends = krt.JoinCollection(derivedBackends, krt.WithName("DerivedBackends"))
	}
	return ret
}

// Plugins registers all built-in policy plugins
func Plugins(agw *AgwCollections) []AgwPlugin {
	return []AgwPlugin{
		NewAgentPlugin(agw),
		NewInferencePlugin(agw),
		NewA2APlugin(agw),
		NewBackendTLSPlugin(agw),
	}
}
