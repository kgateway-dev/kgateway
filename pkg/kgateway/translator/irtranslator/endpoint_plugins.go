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
// copy-on-write surface. Legacy plugins are adapted by requesting a one-time
// deep-cloned mutable input graph from the resolver.
type EndpointPlugin struct {
	groupKind schema.GroupKind
	name      string
	process   endpointPluginFunc
}

type endpointPluginEntry struct {
	groupKind schema.GroupKind
	name      string
	plugin    endpointPluginFunc
}

func OrderedEndpointPlugins(policies sdk.ContributesPolicies) []EndpointPlugin {
	entries := make([]endpointPluginEntry, 0, len(policies))
	for groupKind, policyPlugin := range policies {
		var plugin endpointPluginFunc
		switch {
		case policyPlugin.PerClientEditEndpoints != nil:
			edit := policyPlugin.PerClientEditEndpoints
			plugin = func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniquelyConnectedClient, out *endpoints.EndpointInputsResolver) uint64 {
				return edit(kctx, ctx, ucc, out)
			}
		case policyPlugin.PerClientProcessEndpoints != nil: //nolint:staticcheck // compatibility boundary for legacy plugins
			legacy := policyPlugin.PerClientProcessEndpoints //nolint:staticcheck // isolated below through LegacyMutableInputs
			plugin = func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniquelyConnectedClient, out *endpoints.EndpointInputsResolver) uint64 {
				return legacy(kctx, ctx, ucc, out.LegacyMutableInputs())
			}
		default:
			continue
		}
		entries = append(entries, endpointPluginEntry{
			groupKind: groupKind,
			name:      policyPlugin.Name,
			plugin:    plugin,
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
