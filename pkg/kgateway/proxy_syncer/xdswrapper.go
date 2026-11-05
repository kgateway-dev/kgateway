package proxy_syncer

import (
	"encoding/json"
	"fmt"
	"slices"

	udpaannontations "github.com/cncf/xds/go/udpa/annotations"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/anypb"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
)

var UseDetailedUnmarshalling = !envutils.IsEnvTruthy("DISABLE_DETAILED_SNAP_UNMARSHALLING")

type XdsSnapWrapper struct {
	snap *envoycache.Snapshot
	// erroredClusters lists clusters whose translation failed; resolution never
	// carries them from the published snapshot. Sorted.
	erroredClusters []string
	// +noKrtEquals
	proxyKey string
	// missingReferenced lists referenced clusters absent from CDS (lagging or
	// gone). Sorted.
	missingReferenced []string
	// missingEndpointsReferenced lists referenced EDS clusters whose CLA is a
	// synthesized placeholder, so their endpoints are unknown (see
	// findMissingReferencedEndpointResources). Derived empty CLAs are excluded
	// (#14352). Sorted.
	missingEndpointsReferenced []string
	// synthesizedEndpoints lists placeholder CLAs, referenced or not, whose rows
	// are on their way; settleSynthesizedEndpoints decides which the client
	// receives. Source-less placeholders are steady state and excluded. Sorted.
	synthesizedEndpoints []string
	// expiredCarry lists clusters whose carry hit the warm withhold bound, so a
	// held flip's closure must not restore them. Set only on the gate's copy,
	// never on a KRT row. Sorted.
	// +noKrtEquals
	expiredCarry []string
	// endpointHashes holds each assignment's version contribution for carry-forward
	// (eds_set_version.go). Equals compares their folded EDS version.
	// +noKrtEquals
	endpointHashes map[string]uint64
	// scopesClusters is set in REFERENCED mode, where a published listener can
	// name a cluster scoping left out of CDS. The mode is fixed for the process.
	// +noKrtEquals
	scopesClusters bool
	// translatedClusters is the client's translated clusters before scoping
	// filters them, shared read-only with the per-client cluster row. The
	// de-reference grace keeps only clusters still in it: one that is no longer
	// translated (its backend was deleted, or its translation failed) leaves at
	// once, as with scoping off. Nil means unknown, and graces every
	// de-referenced cluster.
	// +noKrtEquals a change here alone (a backend added or removed while not
	// emitted) changes nothing published; at worst a graced cluster whose backend
	// is then deleted stays until its grace ends.
	translatedClusters map[string]envoycachetypes.ResourceWithTTL
	// translatedEndpoints are the client's endpoints before scoping filters
	// them, with their row hashes, shared read-only with the per-client
	// endpoint row. Set only on a scoped build missing a referenced cluster: a
	// missing tunnel row hides the listener that keeps its proxy cluster, so the
	// build leaves out a cluster it still translates, and the carry takes that
	// cluster's current copy instead of its published one
	// (resolveDeferredPerCluster).
	// +noKrtEquals compared through translatedEndpointsVersion
	translatedEndpoints map[string]envoycachetypes.ResourceWithTTL
	// +noKrtEquals compared through translatedEndpointsVersion
	translatedEndpointHashes map[string]uint64
	// translatedEndpointsVersion is translatedEndpoints' EDS version, so an
	// endpoint change of a cluster scoping left out reaches the gate while it
	// can be carried, and only then.
	translatedEndpointsVersion string
}

// deferred reports whether some referenced cluster was unready at build
// time. syncXds then resolves the snapshot per cluster (see
// resolveDeferredPerCluster).
func (p XdsSnapWrapper) deferred() bool {
	return len(p.missingReferenced) > 0 || len(p.missingEndpointsReferenced) > 0
}

func (p XdsSnapWrapper) WithSnapshot(snap *envoycache.Snapshot) XdsSnapWrapper {
	p.snap = snap
	return p
}

var _ krt.ResourceNamer = XdsSnapWrapper{}

func (p XdsSnapWrapper) Equals(in XdsSnapWrapper) bool {
	// check that all the versions are the equal
	for i, r := range p.snap.Resources {
		if r.Version != in.snap.Resources[i].Version {
			return false
		}
	}
	// Compare the gap classification directly instead of inferring it from
	// versions: a synthesized empty CLA and a derived empty one are identical
	// protos, yet publish differently. A missed change keeps syncXds holding a
	// flip (unbounded with the budget off) or omitting a now-derived CLA. An
	// errored-from-birth cluster also shrinks missingReferenced.
	//
	// Compare errored names too. A cluster out of CDS, gone from the build or
	// filtered out by scoping, moves no version when it errors, yet a held flip
	// or grace may still carry it; only a new event lets the carry drop it.
	// Error text alone is ignored.
	return slices.Equal(p.missingReferenced, in.missingReferenced) &&
		slices.Equal(p.missingEndpointsReferenced, in.missingEndpointsReferenced) &&
		slices.Equal(p.synthesizedEndpoints, in.synthesizedEndpoints) &&
		slices.Equal(p.erroredClusters, in.erroredClusters) &&
		p.translatedEndpointsVersion == in.translatedEndpointsVersion
}

func (p XdsSnapWrapper) ResourceName() string {
	return p.proxyKey
}

// note: this is feature gated, as i'm not confident the new logic can't panic, in all envoy configs
// once 1.18 is out, we can remove the feature gate.
func (p XdsSnapWrapper) MarshalJSON() (out []byte, err error) {
	if !UseDetailedUnmarshalling {
		// use a new struct to prevent infinite recursion
		return json.Marshal(struct {
			Snap     *envoycache.Snapshot
			ProxyKey string
		}{
			Snap:     p.snap,
			ProxyKey: p.proxyKey,
		})
	}

	snap := xds.CloneSnap(p.snap)

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic handling snapshot: %v", r)
		}
	}()

	// redact things
	redact(snap)
	snapJson := map[string]map[string]any{}
	addToSnap(snapJson, "Listeners", snap.Resources[envoycachetypes.Listener].Items)
	addToSnap(snapJson, "Clusters", snap.Resources[envoycachetypes.Cluster].Items)
	addToSnap(snapJson, "Routes", snap.Resources[envoycachetypes.Route].Items)
	addToSnap(snapJson, "Endpoints", snap.Resources[envoycachetypes.Endpoint].Items)

	return json.Marshal(struct {
		Snap     any
		ProxyKey string
	}{
		Snap:     snapJson,
		ProxyKey: p.proxyKey,
	})
}

func addToSnap(snapJson map[string]map[string]any, k string, resources map[string]envoycachetypes.ResourceWithTTL) {
	for rname, r := range resources {
		rJson, _ := protojson.MarshalOptions{UseProtoNames: true}.Marshal(r.Resource)
		var rAny any
		json.Unmarshal(rJson, &rAny)
		if snapJson[k] == nil {
			snapJson[k] = map[string]any{}
		}
		snapJson[k][rname] = rAny
	}
}

func redact(snap *envoycache.Snapshot) {
	// clusters and listener might have secrets
	for _, l := range snap.Resources[envoycachetypes.Listener].Items {
		redactProto(l.Resource)
	}
	for _, l := range snap.Resources[envoycachetypes.Cluster].Items {
		redactProto(l.Resource)
	}
}

func redactProto(m proto.Message) {
	var msg proto.Message = m
	visitFields(msg.ProtoReflect(), false)
}

func isSensitive(fd protoreflect.FieldDescriptor) bool {
	opts := fd.Options().(*descriptorpb.FieldOptions)
	if !proto.HasExtension(opts, udpaannontations.E_Sensitive) {
		return false
	}

	maybeExt := proto.GetExtension(opts, udpaannontations.E_Sensitive)
	return maybeExt.(bool)
}

func visitFields(msg protoreflect.Message, ancestor_sensitive bool) {
	msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		sensitive := ancestor_sensitive || isSensitive(fd)

		if fd.IsList() {
			list := v.List()
			for i := 0; i < list.Len(); i++ {
				elem := list.Get(i)
				if fd.Message() != nil {
					visitMessage(elem, sensitive)
				} else {
					// Redact scalar fields if needed
					if sensitive {
						list.Set(i, redactValue(fd, elem))
					}
				}
			}
		} else if fd.IsMap() {
			m := v.Map()
			m.Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
				if fd.MapValue().Message() != nil {
					visitMessage(v, sensitive)
				} else {
					// Redact scalar fields if needed
					if sensitive {
						m.Set(k, redactValue(fd.MapValue(), v))
					}
				}
				return true
			})
		} else {
			if fd.Message() != nil {
				visitMessage(v, sensitive)
			} else {
				// Redact scalar fields if needed
				if sensitive {
					msg.Set(fd, redactValue(fd, v))
				}
			}
		}
		return true
	})
}

func visitMessage(v protoreflect.Value, sensitive bool) {
	msg := v.Message()
	m := msg.Interface()
	anymsg, ok := m.(*anypb.Any)
	if !ok {
		visitFields(msg, sensitive)
		return
	}

	// special any handling - deserialize it, visit it and write it back.
	newMsg, _ := anymsg.UnmarshalNew()
	visitFields(newMsg.ProtoReflect(), sensitive)
	a, _ := utils.MessageToAny(newMsg)
	anymsg.Value = a.Value
}

func redactValue(fd protoreflect.FieldDescriptor, v protoreflect.Value) protoreflect.Value {
	switch fd.Kind() {
	case protoreflect.StringKind:
		return protoreflect.ValueOfString("[REDACTED]")
	case protoreflect.BytesKind:
		return protoreflect.ValueOfBytes([]byte("[REDACTED]"))
	}
	return v
}
