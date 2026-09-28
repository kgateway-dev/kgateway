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
	// erroredClusters lists clusters whose current translation failed. The
	// publish-time per-cluster resolution treats them fail-closed: they are
	// never resurrected from the previously-published snapshot (see
	// resolveDeferredPerCluster).
	// +noKrtEquals
	erroredClusters []string
	// +noKrtEquals
	proxyKey string
	// deferred marks a snapshot built while some referenced cluster was not
	// ready (see snapshotPerClient's guards). syncXds resolves it per cluster
	// against the currently-published snapshot: previously-published clusters
	// are carried forward, previously-referenced clusters whose CLA row
	// vanished publish the synthesized empty (their slices are gone — that is
	// the truth), and a route flip onto a newly-referenced not-yet-derived
	// cluster is held back for at most the publish budget (see publishGate).
	// +noKrtEquals (derived: true iff either missing list below is non-empty, and Equals compares both)
	deferred bool
	// missingReferenced lists referenced clusters absent from this snapshot's
	// CDS (translation lagging, or the backend is gone). Sorted.
	missingReferenced []string
	// missingEndpointsReferenced lists referenced EDS clusters whose CLA was
	// not derived by the per-client endpoints collection; a synthesized empty
	// stands in for it in the snapshot, and whether the backend has endpoints
	// is unknown (per-client derivation lag, or a plugin that contributed an
	// EDS cluster without an endpoints row; kube Services always derive a
	// row, even sliceless ones like ExternalName). A derived-but-empty CLA
	// is the backend's known truth and is NOT listed (#14352). Sorted.
	missingEndpointsReferenced []string
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
	// The gap classification must be compared explicitly: the per-type
	// versions cannot distinguish a synthesized empty CLA from a derived-but-
	// empty one (they are byte-identical protos, so the recomputed EDS version
	// hash is unchanged when one replaces the other). Without this, a wrapper
	// can transition deferred->ready with every version equal, KRT suppresses
	// the event, and syncXds keeps holding a route flip whose blocking cluster
	// has already derived its (empty) truth — unbounded when the publish
	// budget is disabled. The same applies to a referenced cluster arriving
	// errored-from-birth (missingReferenced shrinks, no version changes).
	// erroredClusters needs no comparison: membership changes always change
	// the CDS version (an errored cluster leaves/enters the cluster items and
	// hash), and error-text-only changes are deliberately suppressed.
	return slices.Equal(p.missingReferenced, in.missingReferenced) &&
		slices.Equal(p.missingEndpointsReferenced, in.missingEndpointsReferenced)
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
