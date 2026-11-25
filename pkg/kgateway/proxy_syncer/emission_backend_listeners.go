package proxy_syncer

import (
	"fmt"
	"maps"
	"slices"

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
// in the gateway's own configuration does, and is returned as a fallback reason
// naming the backend and listener. Claims cover the gateway's routes, not a
// backend's own listeners, so none accounts for it. Only kept backends'
// listeners are walked, so a dropped backend's selector changes nothing; they
// are walked even when kept is already everything translated, so a selector is
// reported either way. The publication gate follows the same listeners for
// readiness, but only to their routing targets (withBackendListenerRoutingTargets).
//
// translated and translatedVersions are the unfiltered set, and kept and
// keptVersions what filterClustersToEmitted retained from it. It returns the
// clusters to emit, their versions, whether that drops any, and the fallback
// reasons, sorted.
func keepBackendListenerTargets(
	translated envoycache.Resources,
	translatedVersions map[string]uint64,
	kept envoycache.Resources,
	keptVersions map[string]uint64,
	backendListeners map[string]baseResources[*envoylistenerv3.Listener],
) (envoycache.Resources, map[string]uint64, bool, []string) {
	var pending []string
	for name := range kept.Items {
		if _, ok := backendListeners[name]; ok {
			pending = append(pending, name)
		}
	}

	items, versions := kept.Items, keptVersions
	cloned := false
	var fallback []string
	for len(pending) > 0 {
		name := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		for _, l := range backendListeners[name].items {
			listener := l.BorrowForRead()
			targets := listenerTargets(listener)
			for _, selector := range targets.RequestTimeSelectors {
				fallback = append(fallback, fmt.Sprintf("backend %q listener %q: %s", name, listener.GetName(), selector))
			}
			for target := range targets.Names {
				item, exists := translated.Items[target]
				if !exists {
					continue
				}
				if _, already := items[target]; already {
					continue
				}
				if !cloned {
					items, versions, cloned = maps.Clone(items), maps.Clone(versions), true
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
	}

	if len(fallback) > 0 {
		slices.Sort(fallback)
		return translated, translatedVersions, false, slices.Compact(fallback)
	}
	if len(items) == len(translated.Items) {
		return translated, translatedVersions, false, nil
	}
	kept.Items = items
	return kept, versions, true, nil
}

// withBackendListenerRoutingTargets returns referenced plus the clusters the
// referenced backends' own listeners route to, followed in turn. A CONNECT
// tunnel's listener sends to the proxy backend's cluster, which no gateway route
// or listener names, so a route to the tunneled backend is not ready until that
// cluster is. It uses the gate's routing-target walk, so a cluster a filter
// calls does not hold routes. referenced is not modified.
func withBackendListenerRoutingTargets(
	referenced map[string]struct{},
	backendListeners map[string]baseResources[*envoylistenerv3.Listener],
) map[string]struct{} {
	var pending []string
	for name := range referenced {
		if _, ok := backendListeners[name]; ok {
			pending = append(pending, name)
		}
	}
	if len(pending) == 0 {
		return referenced
	}
	out := maps.Clone(referenced)
	for len(pending) > 0 {
		name := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		for _, l := range backendListeners[name].items {
			listener := l.BorrowForRead()
			listeners := envoycache.Resources{Items: map[string]envoycachetypes.ResourceWithTTL{listener.GetName(): {Resource: listener}}}
			for target := range collectReferencedClusters(envoycache.Resources{}, listeners) {
				if _, seen := out[target]; seen {
					continue
				}
				out[target] = struct{}{}
				if _, ok := backendListeners[target]; ok {
					pending = append(pending, target)
				}
			}
		}
	}
	return out
}

// listenerTargets walks one backend listener as emittedClustersFor walks a
// gateway's listeners.
func listenerTargets(listener *envoylistenerv3.Listener) emittedClusters {
	return collectReferencedClustersForEmission(envoycache.Resources{}, envoycache.Resources{
		Items: map[string]envoycachetypes.ResourceWithTTL{listener.GetName(): {Resource: listener}},
	})
}
