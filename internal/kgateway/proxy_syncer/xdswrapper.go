package proxy_syncer

import (
	"encoding/json"
	"fmt"

	udpaannontations "github.com/cncf/xds/go/udpa/annotations"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"

	"istio.io/istio/pkg/kube/krt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/anypb"
)

var (
	UseDetailedUnmarshalling = !envutils.IsEnvTruthy("DISABLE_DETAILED_SNAP_UNMARSHALLING")
)

type XdsSnapWrapper struct {
	snap *envoycache.Snapshot
	// erroredClusters contains clusters that encountered errors during backend translation
	// TODO: this is not used anywhere, we need surface this somewhere
	erroredClusters []string
	proxyKey        string
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
	return true
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
		rJson, _ := protojson.Marshal(r.Resource)
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
					visitMessage(msg, fd, elem, sensitive)
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
					visitMessage(msg, fd.MapValue(), v, sensitive)
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
				visitMessage(msg, fd, v, sensitive)
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

func visitMessage(msg protoreflect.Message, fd protoreflect.FieldDescriptor, v protoreflect.Value, sensitive bool) {
	visitMsg := v.Message()
	var anyMsg proto.Message
	m := visitMsg.Interface()
	if anymsg, ok := m.(*anypb.Any); ok {
		anyMsg, _ = anypb.UnmarshalNew(anymsg, proto.UnmarshalOptions{})
		visitMsg = anyMsg.ProtoReflect()
	}
	visitFields(visitMsg, sensitive)
	if anyMsg != nil {
		anymsg, _ := utils.MessageToAny(anyMsg)
		msg.Set(fd, protoreflect.ValueOf(anymsg.ProtoReflect()))
	}
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
