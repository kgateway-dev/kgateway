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

// emittedClusters is the set of cluster names a gateway's generated
// configuration could name, together with whatever made that set unreliable.
//
// RequestTimeSelectors are destinations the walk cannot see. Any of them that no
// plugin has claimed makes the set unsafe to filter on for this gateway: the
// consumer must emit every cluster rather than prune candidates the data plane
// may still select. See Filterable.
type emittedClusters struct {
	Names map[string]struct{}
	// RequestTimeSelectors are the destination selectors the walk cannot
	// resolve, carried structurally rather than as prose so a plugin's claim can
	// be matched against the selector that plugin owns.
	RequestTimeSelectors []requestTimeSelector
}

// requestTimeSelector identifies one piece of configuration that picks its
// destination per request or connection. Name is the cluster specifier
// plugin's extension name, the header a cluster-header route reads, or what
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
	// TcpProxy's cluster from the connection's SNI.
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

// Filterable reports whether this set can be used to prune CDS, given what
// plugins have claimed. Every request-time selector must be accounted for: one
// that is not can select a cluster named nowhere, and pruning candidates it may
// select does not fail visibly.
func (e emittedClusters) Filterable(claims emissionClaims) bool {
	return len(e.unaccountedSelectors(claims)) == 0
}

// unaccountedSelectors are the request-time selectors no claim covers, which is
// what a log or metric should name: "this gateway is unscoped" is only
// actionable with the selector that caused it.
func (e emittedClusters) unaccountedSelectors(claims emissionClaims) []string {
	var unaccounted []string
	for _, selector := range e.RequestTimeSelectors {
		if claims.accountsFor(selector) {
			continue
		}
		unaccounted = append(unaccounted, selector.String())
	}
	return unaccounted
}

func (e emittedClusters) Equals(in emittedClusters) bool {
	return maps.Equal(e.Names, in.Names) &&
		slices.Equal(e.RequestTimeSelectors, in.RequestTimeSelectors)
}

// collectReferencedClustersForEmission returns every cluster name reachable from
// a gateway's generated routes and listeners.
//
// This is the emission set, and it is deliberately not the set
// collectReferencedClusters computes. That one answers "which clusters must be
// present before a route may be published", so it extracts route targets alone:
// treating an ancillary reference as required would let one plugin bug starve a
// whole gateway. This one answers "which clusters may Envoy need", where an
// ancillary cluster — an ext_authz or ext_proc server, a rate limit service, an
// access-log gRPC sink, a JWKS source — is a real cluster whose absence is a
// permanent, route-invisible outage. jwt.go, for instance, resolves its JWKS
// target through GetBackendFromRef and emits backend.ClusterName(): an ordinary
// per-client backend cluster, named only from inside a typed_config chain.
//
// It therefore extracts every string scalar from every message the walk reaches,
// rather than reading named fields of known message types. The bias is
// asymmetric on purpose: over-collection emits one cluster nothing uses, while
// under-collection removes one the data plane needs. Strings that name no
// cluster cost nothing, because callers intersect this set with the clusters
// that actually exist.
//
// wellknown.BlackholeClusterName is always included: routes whose backends fail
// resolution target it, and it may be named by no proto in a healthy build.
//
// The second half of the result is the request-time-destination guard; see
// collectRequestTimeSelectors, matched against plugin claims by
// emittedClusters.Filterable.
func collectReferencedClustersForEmission(routes, listeners envoycache.Resources) emittedClusters {
	out := emittedClusters{Names: map[string]struct{}{
		wellknown.BlackholeClusterName: {},
	}}

	selectors := make(map[requestTimeSelector]struct{})
	visit := func(msg proto.Message) {
		extractEmissionClusterCandidates(msg, out.Names)
		collectRequestTimeSelectors(msg, selectors)
	}
	walkResourceProtos(routes, visit)
	walkResourceProtos(listeners, visit)

	out.RequestTimeSelectors = make([]requestTimeSelector, 0, len(selectors))
	for selector := range selectors {
		out.RequestTimeSelectors = append(out.RequestTimeSelectors, selector)
	}
	slices.SortFunc(out.RequestTimeSelectors, func(a, b requestTimeSelector) int {
		if c := cmp.Compare(a.Kind, b.Kind); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})

	return out
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

// collectRequestTimeSelectors records configuration that chooses a
// destination cluster at request or connection time, from a set the
// configuration never enumerates.
//
// A cluster-header route reads the name from a request header. A cluster
// specifier plugin, referenced or inline, runs a script that typically composes
// a name from a prefix, a request value and a port, so no candidate appears in
// the generated protos at all — only a declarative fallback, if the plugin has
// one. Widening the walk cannot reach these: the prefix is a fragment inside one
// string blob, not a name. The same holds for a weighted-cluster entry or a
// mirror policy that reads its cluster from a header, and for a TcpProxy whose
// cluster a network filter overrides through filter state (the sni_cluster
// filter, or set_filter_state writing envoy.tcp_proxy.cluster). Code does it
// too: a Lua script's httpCall or a Wasm plugin's HTTP or gRPC call names its
// cluster inside the script or the module, and the walk sees only the script
// as one string.
//
// Detection matters more than it first appears. Such a plugin usually checks
// whether its computed cluster exists and falls back when it does not, so
// pruning a candidate does not produce a visible 503 — every affected request
// quietly lands on the fallback instead. Reporting the selector is what turns
// silent misrouting into a visible, metric-bearing loss of the optimization,
// and it is what a plugin claim is matched against: a selector some plugin has
// accounted for stops forcing its gateway back to emitting everything.
//
// Every arm of the cluster_specifier oneof is classified explicitly, and an arm
// this code does not know is reported too: a go-control-plane bump that adds a
// request-time arm must not silently prune its candidates. Over-reporting only
// costs the optimization; TestRouteActionClusterSpecifierArmsAreClassified
// makes a new arm a deliberate decision.
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
		// The walk hands over only typed configs it could not decode; any
		// cluster they name is invisible to it.
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

// walkResourceProtos visits every message reachable from resources, descending
// into nested messages, lists, maps, and typed_config (anypb.Any) payloads, and
// calls visit on each. An Any whose payload cannot be decoded is visited
// itself, in place of the payload.
//
// This broader traversal is used for emission; readiness uses the targeted
// route/listener walk in collectReferencedClusters.
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
			// A typed config whose Go type is not linked into this binary (or
			// that is malformed) can name clusters the walk cannot see. Hand
			// the opaque Any itself to visit, so the emission guard reports it
			// (collectRequestTimeSelectors) rather than pruning what it names.
			logger.Debug("typed_config not decodable during cluster reference scan", "type_url", anyMsg.GetTypeUrl(), "error", err)
			visit(anyMsg)
			return
		}
		walkProto(nestedMsg, visit)
		return
	}

	walkProto(msg.Interface(), visit)
}
