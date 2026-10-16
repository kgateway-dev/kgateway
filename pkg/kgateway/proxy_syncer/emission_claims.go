package proxy_syncer

import (
	"slices"
	"strings"

	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
)

// emissionClaims is every plugin's ClusterEmissionClaim, resolved once at
// startup into the three questions the emission path actually asks: is this
// selector accounted for, is this cluster claimed, and how broad are the claims.
//
// Claims are plugin-global rather than per-gateway. A plugin's selector only
// appears in a gateway whose routes use it, so applying the claim wherever that
// selector appears is already per-gateway in effect, and a gateway that never
// uses the plugin is untouched.
type emissionClaims struct {
	// accountedSelectors are cluster specifier plugin extension names some
	// plugin claimed. A gateway keeps scoping when every request-time selector
	// in its configuration is in here.
	accountedSelectors map[string]struct{}
	names              map[string]struct{}
	prefixes           []string
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
// request, the failure claims exist to prevent. Such a claim's selectors stay
// unaccounted, so their gateways stay unscoped; its valid names and prefixes
// are still kept.
func collectEmissionClaims(policies sdk.ContributesPolicies) emissionClaims {
	claims := emissionClaims{
		accountedSelectors: map[string]struct{}{},
		names:              map[string]struct{}{},
	}
	for gk, policyPlugin := range policies {
		if policyPlugin.ClaimEmittedClusters == nil {
			continue
		}
		claim := policyPlugin.ClaimEmittedClusters()
		destinations, droppedPrefix := 0, false
		for _, name := range claim.Names {
			if name == "" {
				continue
			}
			claims.names[name] = struct{}{}
			destinations++
		}
		for _, prefix := range claim.NamePrefixes {
			if prefix == "" {
				logger.Error("cluster emission claim declares an empty name prefix; ignoring it, since it would re-admit every cluster while still reporting this gateway as scoped",
					"plugin", policyPlugin.Name, "group", gk.Group, "kind", gk.Kind)
				droppedPrefix = true
				continue
			}
			claims.prefixes = append(claims.prefixes, prefix)
			destinations++
		}
		if len(claim.Selectors) > 0 && (destinations == 0 || droppedPrefix) {
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
			claims.accountedSelectors[selector] = struct{}{}
		}
	}
	slices.Sort(claims.prefixes)
	claims.prefixes = slices.Compact(claims.prefixes)
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
		_, ok := c.accountedSelectors[selector.Name]
		return ok
	default:
		return false
	}
}

// claims reports whether this cluster is one a plugin declared it may route to,
// and so must keep being emitted even though nothing names it.
func (c emissionClaims) claims(cluster string) bool {
	if _, ok := c.names[cluster]; ok {
		return true
	}
	for _, prefix := range c.prefixes {
		if strings.HasPrefix(cluster, prefix) {
			return true
		}
	}
	return false
}

func (c emissionClaims) empty() bool {
	return len(c.accountedSelectors) == 0 && len(c.names) == 0 && len(c.prefixes) == 0
}
