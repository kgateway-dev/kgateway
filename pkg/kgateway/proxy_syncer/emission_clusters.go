package proxy_syncer

import (
	"fmt"
	"maps"
	"slices"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// emittedClusters is the set of cluster names a gateway's generated
// configuration could name, together with whatever made that set unreliable.
//
// Unresolvable is the reason list for request-time destinations the walk cannot
// see (see collectReferencedClustersForEmission). A non-empty list means the set
// is not a safe basis for filtering CDS for this gateway: a consumer must fall
// back to emitting every cluster rather than pruning candidates the data plane
// may still select.
type emittedClusters struct {
	Names        map[string]struct{}
	Unresolvable []string
}

// Filterable reports whether this set can be used to prune CDS.
func (e emittedClusters) Filterable() bool { return len(e.Unresolvable) == 0 }

func (e emittedClusters) Equals(in emittedClusters) bool {
	return maps.Equal(e.Names, in.Names) && slices.Equal(e.Unresolvable, in.Unresolvable)
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
// collectUnresolvableSelectors.
func collectReferencedClustersForEmission(routes, listeners envoycache.Resources) emittedClusters {
	out := emittedClusters{Names: map[string]struct{}{
		wellknown.BlackholeClusterName: {},
	}}

	unresolvable := make(map[string]struct{})
	visit := func(msg proto.Message) {
		extractEmissionClusterCandidates(msg, out.Names)
		collectUnresolvableSelectors(msg, unresolvable)
	}
	walkResourceProtos(routes, visit)
	walkResourceProtos(listeners, visit)

	out.Unresolvable = make([]string, 0, len(unresolvable))
	for reason := range unresolvable {
		out.Unresolvable = append(out.Unresolvable, reason)
	}
	slices.Sort(out.Unresolvable)

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

// collectUnresolvableSelectors records route actions that choose their
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
// silent misrouting into a visible, metric-bearing loss of the optimization.
//
// All three oneof arms are named explicitly rather than matched by exclusion,
// so a new declarative arm does not silently disable filtering, and so the
// inline arm — the easiest to overlook — cannot be dropped by accident.
func collectUnresolvableSelectors(msg proto.Message, unresolvable map[string]struct{}) {
	action, ok := msg.(*envoyroutev3.RouteAction)
	if !ok {
		return
	}

	switch action.GetClusterSpecifier().(type) {
	case *envoyroutev3.RouteAction_ClusterHeader:
		unresolvable[fmt.Sprintf("cluster_header %q", action.GetClusterHeader())] = struct{}{}
	case *envoyroutev3.RouteAction_ClusterSpecifierPlugin:
		unresolvable[fmt.Sprintf("cluster_specifier_plugin %q", action.GetClusterSpecifierPlugin())] = struct{}{}
	case *envoyroutev3.RouteAction_InlineClusterSpecifierPlugin:
		name := action.GetInlineClusterSpecifierPlugin().GetExtension().GetName()
		unresolvable[fmt.Sprintf("inline_cluster_specifier_plugin %q", name)] = struct{}{}
	}
}
