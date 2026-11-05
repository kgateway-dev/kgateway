package proxy_syncer

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"k8s.io/apimachinery/pkg/runtime/schema"

	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
)

// emissionClaims is every plugin's ClusterEmissionClaim, resolved at startup
// into selector coverage and kept destinations. A claim's destinations are kept
// only in gateways that use one of its selectors, so controller-wide claims do
// not widen every scoped gateway.
type emissionClaims struct {
	// bySelector maps each claimed cluster specifier plugin extension name to
	// the claims on it.
	bySelector map[string]*selectorClaims
}

// selectorClaims are the claims on one extension name.
type selectorClaims struct {
	// static merges the claims without DestinationsFor, or is nil. They account
	// for every use of the name.
	static *claimedDestinations
	// deriving are the claims with DestinationsFor, in plugin order. They
	// decide each use (deriveUse).
	deriving []derivingClaim
}

// derivingClaim is a claim with DestinationsFor.
type derivingClaim struct {
	plugin string
	group  schema.GroupKind
	derive func(sdk.SelectorUse) sdk.ClaimedDestinations
	// names and prefixes are the claim's own Names and NamePrefixes, kept only
	// with a use derive claims.
	names, prefixes []string
}

// claimedDestinations are the exact names and prefixes kept for a selector.
type claimedDestinations struct {
	names    map[string]struct{}
	prefixes []string
}

// Reasons a use of a claimed selector stays unaccounted, appended to the
// selector in unaccountedSelectors. Each points at a plugin bug or a name
// collision, not at a missing claim.
const (
	claimDerivedNothing      = "claim derived no destinations"
	claimDerivedEmptyPrefix  = "claim derived an empty prefix"
	claimPanicked            = "claim panicked"
	claimConfigUndecodable   = "typed_config does not decode"
	claimReferenceUnresolved = "reference does not resolve"
	claimNotEvaluated        = "claim not evaluated"
)

// collectEmissionClaims gathers claims from every contributed policy plugin;
// most register none.
//
// An empty prefix is dropped with a log: it would admit every cluster while the
// gateway still reports scoped. A claim with a dropped prefix accounts for
// nothing. A static claim accounts for its selectors only with at least one
// name or prefix. Otherwise its selectors stay unaccounted and gateways using
// them stay unscoped, since keeping none of a selector's candidates would prune
// exactly the clusters its plugin computes, and the plugin would silently fall
// back. A claim with DestinationsFor needs no static destinations: it decides
// each use itself.
//
// A claim with no selectors keeps nothing. A cluster a plugin always needs
// belongs in its translation pass's ResourcesToAdd, which scoping never
// filters.
func collectEmissionClaims(policies sdk.ContributesPolicies) emissionClaims {
	claims := emissionClaims{bySelector: map[string]*selectorClaims{}}
	claimedBy := make(map[string]schema.GroupKind)
	for _, gk := range slices.SortedFunc(maps.Keys(policies), compareGroupKind) {
		policyPlugin := policies[gk]
		if policyPlugin.ClaimEmittedClusters == nil {
			continue
		}
		claim := policyPlugin.ClaimEmittedClusters()
		deriving := claim.DestinationsFor != nil
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
			if len(names) > 0 || len(prefixes) > 0 || deriving {
				logger.Error("cluster emission claim names no selectors; its destinations are kept only in gateways that use its selectors, so it keeps nothing",
					"plugin", policyPlugin.Name, "group", gk.Group, "kind", gk.Kind)
			}
			continue
		}
		if droppedPrefix {
			logger.Error("cluster emission claim has a rejected prefix, so it accounts for nothing; gateways using its selectors stay unscoped",
				"plugin", policyPlugin.Name, "group", gk.Group, "kind", gk.Kind, "selectors", claim.Selectors)
			continue
		}
		if !deriving && len(names)+len(prefixes) == 0 {
			logger.Error("cluster emission claim names selectors but neither destinations nor DestinationsFor; its selectors stay unaccounted, so gateways using them stay unscoped",
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
			sc := claims.bySelector[selector]
			if sc == nil {
				sc = &selectorClaims{}
				claims.bySelector[selector] = sc
			}
			if deriving {
				sc.deriving = append(sc.deriving, derivingClaim{
					plugin: policyPlugin.Name, group: gk, derive: claim.DestinationsFor,
					names: names, prefixes: prefixes,
				})
				continue
			}
			if sc.static == nil {
				sc.static = &claimedDestinations{names: map[string]struct{}{}}
			}
			for _, name := range names {
				sc.static.names[name] = struct{}{}
			}
			sc.static.prefixes = append(sc.static.prefixes, prefixes...)
		}
	}
	for _, sc := range claims.bySelector {
		if sc.static != nil {
			slices.Sort(sc.static.prefixes)
			sc.static.prefixes = slices.Compact(sc.static.prefixes)
		}
	}
	return claims
}

func compareGroupKind(a, b schema.GroupKind) int {
	return cmp.Or(cmp.Compare(a.Group, b.Group), cmp.Compare(a.Kind, b.Kind))
}

// claimable reports whether a selector kind can be claimed at all. Only
// cluster specifier plugins, referenced or inline, can be, by extension name. A
// cluster header is whatever the client sends, so no claim can bound it. No
// claim describes the other kinds (sni_cluster, filter state, undecodable typed
// configs, unknown specifiers).
func claimable(selector requestTimeSelector) bool {
	return selector.Kind == selectorSpecifierPlugin || selector.Kind == selectorInlinePlugin
}

// accountsFor reports whether a static claim covers every use of this selector.
func (c emissionClaims) accountsFor(selector requestTimeSelector) bool {
	if !claimable(selector) {
		return false
	}
	sc := c.bySelector[selector.Name]
	return sc != nil && sc.static != nil
}

// derives reports whether a claim with DestinationsFor decides this selector's
// uses.
func (c emissionClaims) derives(selector requestTimeSelector) bool {
	if !claimable(selector) {
		return false
	}
	sc := c.bySelector[selector.Name]
	return sc != nil && len(sc.deriving) > 0
}

// claimedUse is what the deriving claims decided for one selector use.
type claimedUse struct {
	// Names and Prefixes are kept with the use: what each claiming function
	// returned plus that claim's own lists, sorted and deduplicated.
	Names    []string
	Prefixes []string
	// Failure says why no deriving claim claimed the use. It is empty when one
	// did, or when no deriving claim covers the selector.
	Failure string
}

func (u claimedUse) claimed() bool {
	return u.Failure == "" && len(u.Names)+len(u.Prefixes) > 0
}

func (u claimedUse) equal(o claimedUse) bool {
	return u.Failure == o.Failure && slices.Equal(u.Names, o.Names) && slices.Equal(u.Prefixes, o.Prefixes)
}

func (u claimedUse) destinations() *claimedDestinations {
	names := make(map[string]struct{}, len(u.Names))
	for _, name := range u.Names {
		names[name] = struct{}{}
	}
	return &claimedDestinations{names: names, prefixes: u.Prefixes}
}

// deriveClaims records, for each cluster specifier use, what the deriving
// claims decide. Uses are distinct by config, so each claim runs once per
// distinct config in the build, not once per route.
func (e *emittedClusters) deriveClaims(claims emissionClaims) {
	for i := range e.SpecifierUses {
		e.SpecifierUses[i].Derived = claims.deriveUse(e.SpecifierUses[i].specifierUseKey)
	}
}

// deriveUse runs the deriving claims on one use. The use is claimed if any of
// them returns a destination. It then keeps what each claiming function
// returned plus that claim's own lists. A claim that panics, returns an empty
// prefix or returns nothing does not claim the use; that is logged, and the
// reason is kept so the unscoped gateway names it.
func (c emissionClaims) deriveUse(use specifierUseKey) claimedUse {
	if !c.derives(use.Selector) {
		return claimedUse{}
	}
	if use.Unresolved {
		return claimedUse{Failure: claimReferenceUnresolved}
	}
	var names, prefixes, failures []string
	for _, claim := range c.bySelector[use.Selector.Name].deriving {
		config, err := use.decode()
		if err != nil {
			logger.Error("cluster specifier plugin typed_config does not decode; the claim cannot decide this use, so the gateway stays unscoped",
				"selector", use.Selector.String(), "type_url", use.ConfigType, "error", err)
			return claimedUse{Failure: claimConfigUndecodable}
		}
		got, panicked := callDestinationsFor(claim.derive, sdk.SelectorUse{Name: use.Selector.Name, Config: config})
		var failure string
		switch {
		case panicked != nil:
			failure = claimPanicked
			logger.Error("cluster emission claim's DestinationsFor panicked; treating the use as unclaimed, so the gateway stays unscoped",
				"selector", use.Selector.String(), "plugin", claim.plugin, "group", claim.group.Group, "kind", claim.group.Kind, "panic", panicked)
		case slices.Contains(got.NamePrefixes, ""):
			failure = claimDerivedEmptyPrefix
			logger.Error("cluster emission claim's DestinationsFor returned an empty prefix, which would re-admit every cluster; treating the use as unclaimed, so the gateway stays unscoped",
				"selector", use.Selector.String(), "plugin", claim.plugin, "group", claim.group.Group, "kind", claim.group.Kind)
		case !slices.ContainsFunc(got.Names, func(n string) bool { return n != "" }) && len(got.NamePrefixes) == 0:
			failure = claimDerivedNothing
			logger.Error("cluster emission claim's DestinationsFor derived no destinations: the use is not one the plugin generated, or the plugin has a bug; the gateway stays unscoped",
				"selector", use.Selector.String(), "plugin", claim.plugin, "group", claim.group.Group, "kind", claim.group.Kind)
		}
		if failure != "" {
			failures = append(failures, failure)
			continue
		}
		for _, name := range got.Names {
			if name != "" {
				names = append(names, name)
			}
		}
		names = append(names, claim.names...)
		prefixes = append(prefixes, got.NamePrefixes...)
		prefixes = append(prefixes, claim.prefixes...)
	}
	if len(names)+len(prefixes) == 0 {
		slices.Sort(failures)
		return claimedUse{Failure: strings.Join(slices.Compact(failures), "; ")}
	}
	slices.Sort(names)
	slices.Sort(prefixes)
	return claimedUse{Names: slices.Compact(names), Prefixes: slices.Compact(prefixes)}
}

// callDestinationsFor runs plugin code where snapshots are built. A panic
// there would take xDS down for every gateway, so it is recovered and the use
// treated as unclaimed: unscoped is the safe failure.
func callDestinationsFor(derive func(sdk.SelectorUse) sdk.ClaimedDestinations, use sdk.SelectorUse) (got sdk.ClaimedDestinations, panicked any) {
	defer func() {
		if r := recover(); r != nil {
			panicked = r
		}
	}()
	return derive(use), nil
}

// decode returns a fresh copy of the use's typed_config, or nil if it has none.
func (u specifierUseKey) decode() (proto.Message, error) {
	if u.ConfigType == "" && u.Config == "" {
		return nil, nil
	}
	return (&anypb.Any{TypeUrl: u.ConfigType, Value: []byte(u.Config)}).UnmarshalNew()
}

// keptFor returns the destinations this gateway's claimed uses keep: every
// statically claimed selector it uses, and every use a deriving claim claimed.
// Unused claims retain nothing.
func (c emissionClaims) keptFor(emitted emittedClusters) claimedSet {
	var kept claimedSet
	for _, selector := range emitted.RequestTimeSelectors {
		if !c.accountsFor(selector) {
			continue
		}
		if static := c.bySelector[selector.Name].static; !slices.Contains(kept, static) {
			kept = append(kept, static)
		}
	}
	for _, use := range emitted.SpecifierUses {
		if use.Derived.claimed() {
			kept = append(kept, use.Derived.destinations())
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
