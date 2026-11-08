package irtranslator

import (
	"cmp"
	"context"
	"hash/fnv"
	"slices"

	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/endpoints"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

type endpointPluginFunc func(
	kctx krt.HandlerContext,
	ctx context.Context,
	ucc ir.UniquelyConnectedClient,
	out *endpoints.EndpointInputsResolver,
) uint64

// EndpointPlugin is a normalized internal endpoint hook together with its
// stable policy identity. Editor plugins use the resolver's restricted
// copy-on-write surface.
type EndpointPlugin struct {
	groupKind schema.GroupKind
	name      string
	process   endpointPluginFunc
	// mayApply is the plugin's PerClientEndpoints.MayApply; nil means it may
	// apply to every backend.
	mayApply func(krt.HandlerContext, ir.BackendObjectIR) bool
}

// MayApply reports whether the plugin could contribute to backend for some
// client. Plugins that did not declare PerClientEndpoints.MayApply are assumed
// to apply everywhere. kctx is the base translation's context, so a predicate
// that fetches registers the base's dependency on what it read.
func (p EndpointPlugin) MayApply(kctx krt.HandlerContext, backend ir.BackendObjectIR) bool {
	return p.mayApply == nil || p.mayApply(kctx, backend)
}

type endpointPluginEntry struct {
	groupKind schema.GroupKind
	name      string
	plugin    endpointPluginFunc
	mayApply  func(krt.HandlerContext, ir.BackendObjectIR) bool
}

func OrderedEndpointPlugins(policies sdk.ContributesPolicies) []EndpointPlugin {
	entries := make([]endpointPluginEntry, 0, len(policies))
	for groupKind, policyPlugin := range policies {
		hook := policyPlugin.BackendHooks.PerClientEndpoints
		if hook == nil || hook.Edit == nil {
			continue
		}
		edit := hook.Edit
		plugin := func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniquelyConnectedClient, out *endpoints.EndpointInputsResolver) uint64 {
			return edit(kctx, ctx, ucc, out)
		}
		entries = append(entries, endpointPluginEntry{
			groupKind: groupKind,
			name:      policyPlugin.Name,
			plugin:    plugin,
			mayApply:  hook.MayApply,
		})
	}

	slices.SortStableFunc(entries, func(a, b endpointPluginEntry) int {
		if a.groupKind.Group != b.groupKind.Group {
			return cmp.Compare(a.groupKind.Group, b.groupKind.Group)
		}
		if a.groupKind.Kind != b.groupKind.Kind {
			return cmp.Compare(a.groupKind.Kind, b.groupKind.Kind)
		}
		return cmp.Compare(a.name, b.name)
	})

	endpointPlugins := make([]EndpointPlugin, 0, len(entries))
	for _, entry := range entries {
		endpointPlugins = append(endpointPlugins, EndpointPlugin{
			groupKind: entry.groupKind,
			name:      entry.name,
			process:   entry.plugin,
			mayApply:  entry.mayApply,
		})
	}
	return endpointPlugins
}

// ResolveEndpointInputs applies the ordered endpoint hooks to one client's
// working view and returns the isolated result plus the combined plugin hash.
// Both inline-CLA and EDS translation use this helper so their ownership and
// composition semantics cannot diverge.
func ResolveEndpointInputs(
	kctx krt.HandlerContext,
	ctx context.Context,
	ucc ir.UniquelyConnectedClient,
	inputs endpoints.EndpointsInputs,
	plugins []EndpointPlugin,
) (endpoints.EndpointsInputs, uint64) {
	resolver := endpoints.NewEndpointInputsResolver(inputs)
	hasher := fnv.New64a()
	hasContribution := false
	for _, plugin := range plugins {
		contribution := plugin.process(kctx, ctx, ucc, resolver)
		if contribution == 0 {
			continue
		}
		hasContribution = true
		utils.HashStringField(hasher, plugin.groupKind.Group)
		utils.HashStringField(hasher, plugin.groupKind.Kind)
		utils.HashStringField(hasher, plugin.name)
		utils.HashUint64(hasher, contribution)
	}
	if !hasContribution {
		return resolver.Inputs(), 0
	}
	return resolver.Inputs(), hasher.Sum64()
}
