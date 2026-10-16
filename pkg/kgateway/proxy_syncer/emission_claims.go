package proxy_syncer

import (
	"slices"
	"strings"

	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
)

// emissionClaims is every plugin's ClusterEmissionClaim, resolved once at
// startup into the two questions the emission path asks: is this selector
// accounted for, and which clusters do a gateway's selectors keep.
//
// A claim's destinations belong to its selectors: they are kept only in a
// gateway whose configuration uses one of them. Claims are collected once for
// the whole controller, so keeping them everywhere would widen every scoped
// gateway's emitted set by every plugin's candidates, gateways that never use
// the plugin included. Tied to their selectors, they are per-gateway exactly
// where the plugin is.
type emissionClaims struct {
	// bySelector maps each accounted cluster specifier plugin extension name
	// to the destinations kept for it, merged across the claims naming it. A
	// gateway keeps scoping when every request-time selector in its
	// configuration is in here.
	bySelector map[string]*claimedDestinations
}

// claimedDestinations are the clusters claims keep for a selector: exact names,
// and names starting with one of the prefixes.
type claimedDestinations struct {
	names    map[string]struct{}
	prefixes []string
}

// collectEmissionClaims gathers claims from every contributed policy plugin.
// Plugins that route only where the configuration says -- nearly all of them --
// register nothing and cost nothing here.
//
// An empty prefix is dropped with a log rather than honored. It would admit
// every cluster, which restores emit-all while still reporting the gateway as
// scoped: the one outcome worse than reverting, because it is invisible.
//
// A claim accounts for its selectors only if it also says where they may route:
// at least one name or prefix, and no prefix that had to be dropped. Accounting
// for a selector while keeping none of its candidates would prune exactly the
// clusters it computes, and its plugin would quietly fall back on every
// request, the failure claims exist to prevent. Such a claim contributes
// nothing: its selectors stay unaccounted, so the gateways that use them stay
// unscoped and emit everything anyway.
//
// A claim with no selectors keeps nothing either, since its destinations would
// apply in no gateway. A cluster a plugin needs whatever the routes say belongs
// in the resources its translation pass adds (ResourcesToAdd), which scoping
// never filters.
func collectEmissionClaims(policies sdk.ContributesPolicies) emissionClaims {
	claims := emissionClaims{bySelector: map[string]*claimedDestinations{}}
	for gk, policyPlugin := range policies {
		if policyPlugin.ClaimEmittedClusters == nil {
			continue
		}
		claim := policyPlugin.ClaimEmittedClusters()
		var names, prefixes []string
		droppedPrefix := false
		for _, name := range claim.Names {
			if name != "" {
				names = append(names, name)
			}
		}
		for _, prefix := range claim.NamePrefixes {
			if prefix == "" {
				logger.Error("cluster emission claim declares an empty name prefix; ignoring it, since it would re-admit every cluster while still reporting this gateway as scoped",
					"plugin", policyPlugin.Name, "group", gk.Group, "kind", gk.Kind)
				droppedPrefix = true
				continue
			}
			prefixes = append(prefixes, prefix)
		}
		if len(claim.Selectors) == 0 {
			if len(names) > 0 || len(prefixes) > 0 {
				logger.Error("cluster emission claim names no selectors; its destinations are kept only in gateways that use its selectors, so it keeps nothing",
					"plugin", policyPlugin.Name, "group", gk.Group, "kind", gk.Kind)
			}
			continue
		}
		if len(names)+len(prefixes) == 0 || droppedPrefix {
			logger.Error("cluster emission claim names selectors but no valid destinations; its selectors stay unaccounted, so gateways using them stay unscoped",
				"plugin", policyPlugin.Name, "group", gk.Group, "kind", gk.Kind, "selectors", claim.Selectors)
			continue
		}
		for _, selector := range claim.Selectors {
			if selector == "" {
				logger.Error("cluster emission claim names an empty selector; it accounts for nothing",
					"plugin", policyPlugin.Name, "group", gk.Group, "kind", gk.Kind)
				continue
			}
			kept := claims.bySelector[selector]
			if kept == nil {
				kept = &claimedDestinations{names: map[string]struct{}{}}
				claims.bySelector[selector] = kept
			}
			for _, name := range names {
				kept.names[name] = struct{}{}
			}
			kept.prefixes = append(kept.prefixes, prefixes...)
		}
	}
	for _, kept := range claims.bySelector {
		slices.Sort(kept.prefixes)
		kept.prefixes = slices.Compact(kept.prefixes)
	}
	return claims
}

// accountsFor reports whether a claim covers this request-time selector.
//
// Only cluster specifier plugins, referenced or inline, can be claimed: a claim
// names them by extension name. A cluster-header selector never is, since its
// destination is whatever the client puts in the header, so nothing a plugin
// declares can bound the set of clusters it might name; nor are the other kinds
// (filter-state overrides, undecodable typed configs, unknown specifier kinds),
// which no plugin claim describes.
func (c emissionClaims) accountsFor(selector requestTimeSelector) bool {
	switch selector.Kind {
	case selectorSpecifierPlugin, selectorInlinePlugin:
		_, ok := c.bySelector[selector.Name]
		return ok
	default:
		return false
	}
}

// keptFor returns what the claims keep in a gateway whose configuration has
// these request-time selectors: the destinations of the claimed ones. A gateway
// that uses none of a claim's selectors keeps none of its destinations.
func (c emissionClaims) keptFor(selectors []requestTimeSelector) claimedSet {
	var kept claimedSet
	for _, selector := range selectors {
		if !c.accountsFor(selector) {
			continue
		}
		if destinations := c.bySelector[selector.Name]; !slices.Contains(kept, destinations) {
			kept = append(kept, destinations)
		}
	}
	return kept
}

func (c emissionClaims) empty() bool {
	return len(c.bySelector) == 0
}

// claimedSet is the destinations a gateway's claimed selectors keep.
type claimedSet []*claimedDestinations

// claims reports whether this cluster is one the gateway's claimed selectors
// may route to, and so must keep being emitted even though nothing names it.
func (s claimedSet) claims(cluster string) bool {
	for _, kept := range s {
		if _, ok := kept.names[cluster]; ok {
			return true
		}
		for _, prefix := range kept.prefixes {
			if strings.HasPrefix(cluster, prefix) {
				return true
			}
		}
	}
	return false
}
