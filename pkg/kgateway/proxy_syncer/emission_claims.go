package proxy_syncer

import (
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
)

// emissionClaims is every plugin's ClusterEmissionClaim, resolved at startup
// into selector coverage and kept destinations. A claim's destinations are kept
// only in gateways that use one of its selectors, so controller-wide claims do
// not widen every scoped gateway.
type emissionClaims struct {
	// bySelector maps each accounted cluster specifier plugin extension name to
	// its destinations, merged across claims. A gateway stays scoped only if
	// every request-time selector it uses is here.
	bySelector map[string]*claimedDestinations
}

// claimedDestinations are the exact names and prefixes kept for a selector.
type claimedDestinations struct {
	names    map[string]struct{}
	prefixes []string
}

// collectEmissionClaims gathers claims from every contributed policy plugin;
// most register none.
//
// An empty prefix is dropped with a log: it would admit every cluster while the
// gateway still reports scoped. A claim accounts for its selectors only with at
// least one name or prefix and no dropped prefix. Otherwise its selectors stay
// unaccounted and gateways using them stay unscoped, since keeping none of a
// selector's candidates would prune exactly the clusters its plugin computes,
// and the plugin would silently fall back.
//
// A claim with no selectors keeps nothing. A cluster a plugin always needs
// belongs in its translation pass's ResourcesToAdd, which scoping never
// filters.
func collectEmissionClaims(policies sdk.ContributesPolicies) emissionClaims {
	claims := emissionClaims{bySelector: map[string]*claimedDestinations{}}
	claimedBy := make(map[string]schema.GroupKind)
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
			if other, ok := claimedBy[selector]; ok && other != gk {
				logger.Error("two plugins claim the same cluster specifier extension name; claims match by name alone, so each plugin's destinations are kept wherever either plugin's routes use it. Give each plugin a name it owns",
					"selector", selector, "plugin", policyPlugin.Name, "group", gk.Group, "kind", gk.Kind,
					"other_group", other.Group, "other_kind", other.Kind)
			}
			claimedBy[selector] = gk
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

// accountsFor reports whether a claim covers this selector. Only cluster
// specifier plugins, referenced or inline, can be claimed, by extension name. A
// cluster header is whatever the client sends, so no claim can bound it. No
// claim describes the other kinds (sni_cluster, filter state, undecodable typed
// configs, unknown specifiers).
func (c emissionClaims) accountsFor(selector requestTimeSelector) bool {
	switch selector.Kind {
	case selectorSpecifierPlugin, selectorInlinePlugin:
		_, ok := c.bySelector[selector.Name]
		return ok
	default:
		return false
	}
}

// keptFor returns destinations for claimed selectors used by this gateway.
// Unused claims retain nothing.
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

// claims reports whether one of the gateway's claimed selectors may route to
// cluster, which must then stay emitted though nothing names it.
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
