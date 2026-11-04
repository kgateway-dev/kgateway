package proxy_syncer

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	sfsvalue "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/common/set_filter_state/v3"
	envoysniclusterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/sni_cluster/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// emittedClusters is the cluster names a gateway's configuration could name,
// plus the request-time selectors the walk cannot see. An unclaimed selector
// forces emit-all (see Filterable).
type emittedClusters struct {
	Names map[string]struct{}
	// RequestTimeSelectors are the selectors the walk cannot resolve, kept
	// structured so a claim can match the selector its plugin owns.
	RequestTimeSelectors []requestTimeSelector
	// SpecifierUses are the distinct uses of cluster specifier plugins, with
	// what claims with DestinationsFor decided for each. Sorted.
	SpecifierUses []specifierUse
}

// specifierUseKey identifies one use of a cluster specifier plugin: its
// selector and its typed_config. Uses of one extension name with different
// configs, such as one per namespace, stay apart, so a claim decides each.
type specifierUseKey struct {
	Selector requestTimeSelector
	// ConfigType and Config are the typed_config's type URL and serialized
	// bytes, a fingerprint that needs no re-encoding. Both are empty if the
	// extension carries no typed_config.
	ConfigType string
	Config     string
	// Unresolved marks a reference the route's RouteConfiguration does not
	// define in its cluster_specifier_plugins.
	Unresolved bool
}

// specifierUse is one use and what the deriving claims decided for it
// (deriveClaims).
type specifierUse struct {
	specifierUseKey
	Derived claimedUse
}

// requestTimeSelector is configuration that picks its destination per request
// or connection. Name is the plugin extension name, the header read, or what
// the kind's comment says.
type requestTimeSelector struct {
	Kind string
	Name string
}

const (
	selectorClusterHeader   = "cluster_header"
	selectorSpecifierPlugin = "cluster_specifier_plugin"
	selectorInlinePlugin    = "inline_cluster_specifier_plugin"
	// selectorUnknownSpecifier is a cluster_specifier arm this code does not
	// know; Name is its Go type.
	selectorUnknownSpecifier = "cluster_specifier"
	// selectorSniCluster is the sni_cluster network filter, which sets a
	// TcpProxy's cluster from SNI.
	selectorSniCluster = "sni_cluster"
	// selectorFilterState is a filter state object that overrides a
	// destination; Name is its key.
	selectorFilterState = "filter_state"
	// selectorOpaqueConfig is a typed_config the walk could not decode; Name
	// is its type URL.
	selectorOpaqueConfig = "undecodable typed_config"
	// selectorScript is code that can call any cluster (scriptedConfigs);
	// Name is its message's full name.
	selectorScript = "script"
)

func (s requestTimeSelector) String() string {
	if s.Name == "" {
		return s.Kind
	}
	return s.Kind + " " + strconv.Quote(s.Name)
}

// Filterable reports whether this set can prune CDS given the claims: every
// request-time selector must be accounted for. An unaccounted one can select a
// cluster named nowhere, and pruning it fails invisibly.
func (e emittedClusters) Filterable(claims emissionClaims) bool {
	return len(e.unaccountedSelectors(claims)) == 0
}

// unaccountedSelectors are the selectors no claim covers, named in logs and
// metrics so an unscoped gateway is actionable. A static claim covers every use
// of its selector. Where only claims with DestinationsFor cover it, each use
// must have been claimed; one that was not is named with the reason, which
// tells a plugin bug or name collision apart from a missing claim.
func (e emittedClusters) unaccountedSelectors(claims emissionClaims) []string {
	var unaccounted []string
	for _, selector := range e.RequestTimeSelectors {
		if claims.accountsFor(selector) {
			continue
		}
		if !claims.derives(selector) {
			unaccounted = append(unaccounted, selector.String())
			continue
		}
		var reasons []string
		uses := 0
		for _, use := range e.SpecifierUses {
			if use.Selector != selector {
				continue
			}
			uses++
			if !use.Derived.claimed() {
				reasons = append(reasons, cmp.Or(use.Derived.Failure, claimNotEvaluated))
			}
		}
		if uses == 0 {
			reasons = append(reasons, claimNotEvaluated)
		}
		slices.Sort(reasons)
		for _, reason := range slices.Compact(reasons) {
			unaccounted = append(unaccounted, selector.String()+" ("+reason+")")
		}
	}
	return unaccounted
}

func (e emittedClusters) Equals(in emittedClusters) bool {
	return maps.Equal(e.Names, in.Names) &&
		slices.Equal(e.RequestTimeSelectors, in.RequestTimeSelectors) &&
		slices.EqualFunc(e.SpecifierUses, in.SpecifierUses, func(a, b specifierUse) bool {
			return a.specifierUseKey == b.specifierUseKey && a.Derived.equal(b.Derived)
		})
}

// collectReferencedClustersForEmission returns every cluster name reachable
// from a gateway's generated routes and listeners.
//
// It differs from collectReferencedClusters, which takes route targets only
// because treating an ancillary reference as required would let one plugin bug
// starve a whole gateway. Emission must include ancillary clusters (ext_authz,
// ext_proc, rate limit, access-log sinks, JWKS): a missing one is a permanent,
// route-invisible outage. jwt.go, for instance, names its JWKS cluster only
// inside a typed_config chain.
//
// So it collects every string scalar the walk reaches. Over-collection costs
// one unused cluster; under-collection removes a needed one. Callers intersect
// the result with existing clusters. The blackhole cluster is always included:
// routes with unresolvable backends target it, and a healthy build may never
// name it.
//
// collectRequestTimeSelectors fills RequestTimeSelectors with what the walk
// cannot see, and collectSpecifierUse records each cluster specifier use with
// its config, for claims that decide per use (emittedClustersFor).
func collectReferencedClustersForEmission(routes, listeners envoycache.Resources) emittedClusters {
	out := emittedClusters{Names: map[string]struct{}{
		wellknown.BlackholeClusterName: {},
	}}

	selectors := make(map[requestTimeSelector]struct{})
	uses := make(map[specifierUseKey]struct{})
	// plugins are the cluster_specifier_plugins of the RouteConfiguration being
	// walked. The walk visits a RouteConfiguration before its routes.
	var plugins map[string]*anypb.Any
	visit := func(msg proto.Message) {
		if routeConfig, ok := msg.(*envoyroutev3.RouteConfiguration); ok {
			plugins = clusterSpecifierPluginConfigs(routeConfig)
		}
		extractEmissionClusterCandidates(msg, out.Names)
		collectRequestTimeSelectors(msg, selectors)
		collectSpecifierUse(msg, plugins, uses)
	}
	walkResourceProtos(routes, visit)
	walkResourceProtos(listeners, visit)

	out.RequestTimeSelectors = make([]requestTimeSelector, 0, len(selectors))
	for selector := range selectors {
		out.RequestTimeSelectors = append(out.RequestTimeSelectors, selector)
	}
	slices.SortFunc(out.RequestTimeSelectors, compareSelectors)

	for key := range uses {
		out.SpecifierUses = append(out.SpecifierUses, specifierUse{specifierUseKey: key})
	}
	slices.SortFunc(out.SpecifierUses, func(a, b specifierUse) int {
		return cmp.Or(
			compareSelectors(a.Selector, b.Selector),
			cmp.Compare(a.ConfigType, b.ConfigType),
			cmp.Compare(a.Config, b.Config),
			compareBool(a.Unresolved, b.Unresolved),
		)
	})

	return out
}

func compareSelectors(a, b requestTimeSelector) int {
	return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.Name, b.Name))
}

func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a:
		return -1
	default:
		return 1
	}
}

// clusterSpecifierPluginConfigs maps a RouteConfiguration's cluster specifier
// plugin extension names to their typed_config, which a route references by
// name.
func clusterSpecifierPluginConfigs(routeConfig *envoyroutev3.RouteConfiguration) map[string]*anypb.Any {
	configs := make(map[string]*anypb.Any, len(routeConfig.GetClusterSpecifierPlugins()))
	for _, plugin := range routeConfig.GetClusterSpecifierPlugins() {
		configs[plugin.GetExtension().GetName()] = plugin.GetExtension().GetTypedConfig()
	}
	return configs
}

// collectSpecifierUse records a route's cluster specifier plugin use with its
// typed_config: inline, or resolved against the RouteConfiguration's
// cluster_specifier_plugins. A reference it does not define is unresolved, so
// no claim with DestinationsFor can claim it.
func collectSpecifierUse(msg proto.Message, plugins map[string]*anypb.Any, uses map[specifierUseKey]struct{}) {
	action, ok := msg.(*envoyroutev3.RouteAction)
	if !ok {
		return
	}
	var key specifierUseKey
	switch spec := action.GetClusterSpecifier().(type) {
	case *envoyroutev3.RouteAction_ClusterSpecifierPlugin:
		key.Selector = requestTimeSelector{selectorSpecifierPlugin, spec.ClusterSpecifierPlugin}
		config, defined := plugins[spec.ClusterSpecifierPlugin]
		key.Unresolved = !defined
		key.ConfigType, key.Config = config.GetTypeUrl(), string(config.GetValue())
	case *envoyroutev3.RouteAction_InlineClusterSpecifierPlugin:
		extension := spec.InlineClusterSpecifierPlugin.GetExtension()
		key.Selector = requestTimeSelector{selectorInlinePlugin, extension.GetName()}
		key.ConfigType, key.Config = extension.GetTypedConfig().GetTypeUrl(), string(extension.GetTypedConfig().GetValue())
	default:
		return
	}
	uses[key] = struct{}{}
}

// extractEmissionClusterCandidates collects direct string fields, list elements
// and map keys/values. The outer walk handles nested messages.
func extractEmissionClusterCandidates(msg proto.Message, candidates map[string]struct{}) {
	if msg == nil {
		return
	}
	reflected := msg.ProtoReflect()
	if !reflected.IsValid() {
		return
	}

	reflected.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsMap():
			if fd.MapKey().Kind() != protoreflect.StringKind && fd.MapValue().Kind() != protoreflect.StringKind {
				return true
			}
			v.Map().Range(func(k protoreflect.MapKey, value protoreflect.Value) bool {
				if fd.MapKey().Kind() == protoreflect.StringKind {
					addCandidate(candidates, k.String())
				}
				if fd.MapValue().Kind() == protoreflect.StringKind {
					addCandidate(candidates, value.String())
				}
				return true
			})
		case fd.IsList():
			if fd.Kind() != protoreflect.StringKind {
				return true
			}
			list := v.List()
			for i := 0; i < list.Len(); i++ {
				addCandidate(candidates, list.Get(i).String())
			}
		case fd.Kind() == protoreflect.StringKind:
			addCandidate(candidates, v.String())
		}
		return true
	})
}

func addCandidate(candidates map[string]struct{}, s string) {
	if s == "" {
		return
	}
	candidates[s] = struct{}{}
}

// tcpProxyClusterFilterStateKey names the filter-state override for TcpProxy's cluster.
const tcpProxyClusterFilterStateKey = "envoy.tcp_proxy.cluster"

// collectRequestTimeSelectors records configuration that picks a destination at
// request or connection time from a set the configuration never enumerates: a
// cluster header (on a route, weighted entry or mirror policy), a cluster
// specifier plugin, referenced or inline, or a TcpProxy filter-state override
// (sni_cluster, or set_filter_state writing envoy.tcp_proxy.cluster). A
// specifier script typically composes the name from a prefix, a request value
// and a port, so no walk finds the candidates: the prefix is a fragment of a
// string blob.
//
// Such a plugin usually falls back when its cluster is missing, so pruning a
// candidate misroutes silently. Reporting the selector makes that a visible
// loss of the optimization and gives claims something to match.
//
// Every cluster_specifier arm is classified, and unknown arms are reported, so
// a go-control-plane bump cannot silently prune a new arm's candidates.
// Over-reporting only costs the optimization.
// TestRouteActionClusterSpecifierArmsAreClassified forces the decision.
func collectRequestTimeSelectors(msg proto.Message, selectors map[requestTimeSelector]struct{}) {
	switch m := msg.(type) {
	case *envoyroutev3.RouteAction:
		switch spec := m.GetClusterSpecifier().(type) {
		case nil, *envoyroutev3.RouteAction_Cluster, *envoyroutev3.RouteAction_WeightedClusters:
			// Declarative: the walk sees every name.
		case *envoyroutev3.RouteAction_ClusterHeader:
			selectors[requestTimeSelector{selectorClusterHeader, m.GetClusterHeader()}] = struct{}{}
		case *envoyroutev3.RouteAction_ClusterSpecifierPlugin:
			selectors[requestTimeSelector{selectorSpecifierPlugin, m.GetClusterSpecifierPlugin()}] = struct{}{}
		case *envoyroutev3.RouteAction_InlineClusterSpecifierPlugin:
			name := m.GetInlineClusterSpecifierPlugin().GetExtension().GetName()
			selectors[requestTimeSelector{selectorInlinePlugin, name}] = struct{}{}
		default:
			selectors[requestTimeSelector{selectorUnknownSpecifier, fmt.Sprintf("%T", spec)}] = struct{}{}
		}
	case *envoyroutev3.WeightedCluster_ClusterWeight:
		if header := m.GetClusterHeader(); header != "" {
			selectors[requestTimeSelector{selectorClusterHeader, header}] = struct{}{}
		}
	case *envoyroutev3.RouteAction_RequestMirrorPolicy:
		if header := m.GetClusterHeader(); header != "" {
			selectors[requestTimeSelector{selectorClusterHeader, header}] = struct{}{}
		}
	case *envoysniclusterv3.SniCluster:
		selectors[requestTimeSelector{Kind: selectorSniCluster}] = struct{}{}
	case *sfsvalue.FilterStateValue:
		if m.GetObjectKey() == tcpProxyClusterFilterStateKey {
			selectors[requestTimeSelector{selectorFilterState, tcpProxyClusterFilterStateKey}] = struct{}{}
		}
	case *anypb.Any:
		// The walk passes only undecodable Anys here; it cannot see clusters they name.
		selectors[requestTimeSelector{selectorOpaqueConfig, m.GetTypeUrl()}] = struct{}{}
	default:
		// Matched by name: importing these packages would link them into the
		// controller, and an unlinked one is reported as undecodable above.
		if name := msg.ProtoReflect().Descriptor().FullName(); scriptedConfigs[name] {
			selectors[requestTimeSelector{selectorScript, string(name)}] = struct{}{}
		}
	}
}

// scriptedConfigs are configurations that run code able to call any cluster.
var scriptedConfigs = map[protoreflect.FullName]bool{
	"envoy.extensions.filters.http.lua.v3.Lua":         true,
	"envoy.extensions.filters.http.lua.v3.LuaPerRoute": true,
	"envoy.extensions.wasm.v3.PluginConfig":            true,
}

// walkResourceProtos calls visit on every message reachable from resources,
// including decoded typed_config payloads. An undecodable Any is visited itself
// so the guard can report it. Readiness uses the narrower walk in
// collectReferencedClusters.
func walkResourceProtos(resources envoycache.Resources, visit func(proto.Message)) {
	for _, item := range resources.Items {
		if item.Resource == nil {
			continue
		}
		walkProto(item.Resource, visit)
	}
}

func walkProto(msg proto.Message, visit func(proto.Message)) {
	if msg == nil {
		return
	}
	visit(msg)
	walkNestedProtos(msg.ProtoReflect(), visit)
}

func walkNestedProtos(msg protoreflect.Message, visit func(proto.Message)) {
	if !msg.IsValid() {
		return
	}

	msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsList() && fd.Message() != nil:
			list := v.List()
			for i := 0; i < list.Len(); i++ {
				walkProtoValue(list.Get(i), visit)
			}
		case fd.IsMap() && fd.MapValue().Message() != nil:
			m := v.Map()
			m.Range(func(_ protoreflect.MapKey, value protoreflect.Value) bool {
				walkProtoValue(value, visit)
				return true
			})
		case !fd.IsList() && !fd.IsMap() && fd.Message() != nil:
			walkProtoValue(v, visit)
		}
		return true
	})
}

func walkProtoValue(v protoreflect.Value, visit func(proto.Message)) {
	msg := v.Message()
	if !msg.IsValid() {
		return
	}

	if anyMsg, ok := msg.Interface().(*anypb.Any); ok {
		if anyMsg.GetTypeUrl() == "" && len(anyMsg.GetValue()) == 0 {
			// No payload, so no cluster name. The per-route filter-enable
			// marker is a FilterConfig with an empty Any, which Envoy requires
			// as is; reporting it would leave every such gateway unscoped.
			return
		}
		nestedMsg, err := anyMsg.UnmarshalNew()
		if err != nil {
			// An undecodable typed config (type not linked in, or malformed)
			// can name clusters the walk cannot see. Visit the Any itself so
			// collectRequestTimeSelectors reports it.
			logger.Debug("typed_config not decodable during cluster reference scan", "type_url", anyMsg.GetTypeUrl(), "error", err)
			visit(anyMsg)
			return
		}
		walkProto(nestedMsg, visit)
		return
	}

	walkProto(msg.Interface(), visit)
}
