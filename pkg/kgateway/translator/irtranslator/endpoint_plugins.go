package irtranslator

import (
	"sort"

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

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].groupKind.Group != entries[j].groupKind.Group {
			return entries[i].groupKind.Group < entries[j].groupKind.Group
		}
		if entries[i].groupKind.Kind != entries[j].groupKind.Kind {
			return entries[i].groupKind.Kind < entries[j].groupKind.Kind
		}
		return entries[i].name < entries[j].name
	})

	endpointPlugins := make([]sdk.EndpointPlugin, 0, len(entries))
	for _, entry := range entries {
		endpointPlugins = append(endpointPlugins, entry.plugin)
	}
	return endpointPlugins
}
