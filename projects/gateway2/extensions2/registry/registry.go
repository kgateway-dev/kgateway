package registry

import (
	"context"
	"maps"

	"github.com/solo-io/gloo/projects/gateway2/extensions2/common"
	extensionsplug "github.com/solo-io/gloo/projects/gateway2/extensions2/plugin"
	"github.com/solo-io/gloo/projects/gateway2/extensions2/plugins/directresponse"
	"github.com/solo-io/gloo/projects/gateway2/extensions2/plugins/istio"
	"github.com/solo-io/gloo/projects/gateway2/extensions2/plugins/kubernetes"
	"github.com/solo-io/gloo/projects/gateway2/extensions2/plugins/routepolicy"
	"github.com/solo-io/gloo/projects/gateway2/extensions2/plugins/upstream"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func mergedGw(funcs []extensionsplug.GwTranslatorFactory) extensionsplug.GwTranslatorFactory {
	return func(gw *gwv1.Gateway) extensionsplug.K8sGwTranslator {
		for _, f := range funcs {
			if f == nil {
				continue
			}
			ret := f(gw)
			if ret != nil {
				return ret
			}
		}
		return nil
	}

}

func MergePlugins(plug ...extensionsplug.Plugin) extensionsplug.Plugin {
	ret := extensionsplug.Plugin{
		ContributesPolicies:  make(map[schema.GroupKind]extensionsplug.PolicyPlugin),
		ContributesUpstreams: make(map[schema.GroupKind]extensionsplug.UpstreamPlugin),
	}
	var funcs []extensionsplug.GwTranslatorFactory
	for _, p := range plug {
		maps.Copy(ret.ContributesPolicies, p.ContributesPolicies)
		maps.Copy(ret.ContributesUpstreams, p.ContributesUpstreams)
		funcs = append(funcs, p.ContributesGwTranslator)
	}
	ret.ContributesGwTranslator = mergedGw(funcs)
	return ret
}

func Plugins(ctx context.Context, commoncol common.CommonCollections) []extensionsplug.Plugin {
	return []extensionsplug.Plugin{
		// Add plugins here
		upstream.NewPlugin(ctx, commoncol),
		routepolicy.NewPlugin(ctx, commoncol),
		directresponse.NewPlugin(ctx, commoncol),
		kubernetes.NewPlugin(ctx, commoncol),
		istio.NewPlugin(ctx, commoncol),
	}
}

func AllPlugins(ctx context.Context, commoncol common.CommonCollections) extensionsplug.Plugin {
	return MergePlugins(Plugins(ctx, commoncol)...)
}
