package proxy_syncer

import (
	"maps"

	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
)

// keepBackendListenerTargets adds back the clusters that kept backends' own
// listeners name. A backend can ship listeners with its cluster
// (ProcessBaseClusterResources): a CONNECT tunnel's listener proxies through
// another backend's cluster that no gateway route or listener names, so the
// emission walk never sees it, and pruning it fails every connection through
// the tunnel. The clusters a kept target's own listeners name are kept in turn.
//
// A request-time selector in one of those listeners keeps every cluster, as one
// in the gateway's own configuration does. The targets are not routing targets
// for the publication gate, which reads only the gateway's routes and listeners.
//
// translated and translatedVersions are the unfiltered set, and kept and
// keptVersions what filterClustersToEmitted retained from it. It returns the
// clusters to emit, their versions, and whether that still drops any.
func keepBackendListenerTargets(
	translated envoycache.Resources,
	translatedVersions map[string]uint64,
	kept envoycache.Resources,
	keptVersions map[string]uint64,
	backendListeners map[string]baseResources[*envoylistenerv3.Listener],
) (envoycache.Resources, map[string]uint64, bool) {
	var pending []string
	for name := range kept.Items {
		if _, ok := backendListeners[name]; ok {
			pending = append(pending, name)
		}
	}
	if len(pending) == 0 {
		return kept, keptVersions, true
	}

	items := maps.Clone(kept.Items)
	versions := maps.Clone(keptVersions)
	for len(pending) > 0 {
		name := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		targets := backendListenerTargets(backendListeners[name])
		if !targets.Filterable() {
			return translated, translatedVersions, false
		}
		for target := range targets.Names {
			item, exists := translated.Items[target]
			if !exists {
				continue
			}
			if _, already := items[target]; already {
				continue
			}
			items[target] = item
			if version, ok := translatedVersions[target]; ok {
				versions[target] = version
			}
			if _, ok := backendListeners[target]; ok {
				pending = append(pending, target)
			}
		}
	}

	if len(items) == len(translated.Items) {
		return translated, translatedVersions, false
	}
	kept.Items = items
	return kept, versions, true
}

// backendListenerTargets walks a backend's own listeners as emittedClustersFor
// walks a gateway's.
func backendListenerTargets(listeners baseResources[*envoylistenerv3.Listener]) emittedClusters {
	items := make(map[string]envoycachetypes.ResourceWithTTL, len(listeners.items))
	for _, l := range listeners.items {
		listener := l.BorrowForRead()
		items[listener.GetName()] = envoycachetypes.ResourceWithTTL{Resource: listener}
	}
	return collectReferencedClustersForEmission(envoycache.Resources{}, envoycache.Resources{Items: items})
}
