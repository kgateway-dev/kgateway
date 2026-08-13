package irtranslator

import (
	"cmp"
	"slices"

	"k8s.io/apimachinery/pkg/runtime/schema"

	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
)

type endpointPluginEntry struct {
	groupKind schema.GroupKind
	name      string
	plugin    sdk.EndpointPlugin
}

func OrderedEndpointPlugins(policies sdk.ContributesPolicies) []sdk.EndpointPlugin {
	entries := make([]endpointPluginEntry, 0, len(policies))
	for groupKind, plugin := range policies {
		if plugin.PerClientProcessEndpoints == nil {
			continue
		}
		entries = append(entries, endpointPluginEntry{
			groupKind: groupKind,
			name:      plugin.Name,
			plugin:    plugin.PerClientProcessEndpoints,
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

	endpointPlugins := make([]sdk.EndpointPlugin, 0, len(entries))
	for _, entry := range entries {
		endpointPlugins = append(endpointPlugins, entry.plugin)
	}
	return endpointPlugins
}
