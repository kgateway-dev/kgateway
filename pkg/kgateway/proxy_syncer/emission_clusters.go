package proxy_syncer

import (
	"cmp"
	"maps"
	"slices"
	"strconv"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

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

// requestTimeSelector identifies one route action that picks its destination per
// request. Name is the cluster specifier plugin's extension name, or the header
// a cluster-header route reads.
type requestTimeSelector struct {
	Kind string
	Name string
}

const (
	selectorClusterHeader   = "cluster_header"
	selectorSpecifierPlugin = "cluster_specifier_plugin"
	selectorInlinePlugin    = "inline_cluster_specifier_plugin"
)

func (s requestTimeSelector) String() string { return s.Kind + " " + strconv.Quote(s.Name) }

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

// extractEmissionClusterCandidates adds every string this message carries
// directly: singular fields, repeated elements, and both halves of a map entry.
// Nested messages are reached by the walk, not from here.
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

// collectRequestTimeSelectors records route actions that choose their
// destination at request time, from a set the configuration never enumerates.
//
// A cluster-header route reads the name from a request header. A cluster
// specifier plugin, referenced or inline, runs a script that typically composes
// a name from a prefix, a request value and a port, so no candidate appears in
// the generated protos at all — only a declarative fallback, if the plugin has
// one. Widening the walk cannot reach these: the prefix is a fragment inside one
// string blob, not a name.
//
// Detection matters more than it first appears. Such a plugin usually checks
// whether its computed cluster exists and falls back when it does not, so
// pruning a candidate does not produce a visible 503 — every affected request
// quietly lands on the fallback instead. Reporting the selector is what turns
// silent misrouting into a visible, metric-bearing loss of the optimization,
// and it is what a plugin claim is matched against: a selector some plugin has
// accounted for stops forcing its gateway back to emitting everything.
//
// All three oneof arms are named explicitly rather than matched by exclusion,
// so a new declarative arm does not silently disable filtering, and so the
// inline arm — the easiest to overlook — cannot be dropped by accident.
func collectRequestTimeSelectors(msg proto.Message, selectors map[requestTimeSelector]struct{}) {
	action, ok := msg.(*envoyroutev3.RouteAction)
	if !ok {
		return
	}

	switch action.GetClusterSpecifier().(type) {
	case *envoyroutev3.RouteAction_ClusterHeader:
		selectors[requestTimeSelector{selectorClusterHeader, action.GetClusterHeader()}] = struct{}{}
	case *envoyroutev3.RouteAction_ClusterSpecifierPlugin:
		selectors[requestTimeSelector{selectorSpecifierPlugin, action.GetClusterSpecifierPlugin()}] = struct{}{}
	case *envoyroutev3.RouteAction_InlineClusterSpecifierPlugin:
		name := action.GetInlineClusterSpecifierPlugin().GetExtension().GetName()
		selectors[requestTimeSelector{selectorInlinePlugin, name}] = struct{}{}
	}
}
