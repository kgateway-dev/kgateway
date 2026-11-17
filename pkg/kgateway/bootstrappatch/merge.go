package bootstrappatch

import (
	"slices"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyoverloadv3 "github.com/envoyproxy/go-control-plane/envoy/config/overload/v3"
	"google.golang.org/protobuf/proto"
)

// namedEntry is an entry of a list in which Envoy requires each name to be unique.
type namedEntry interface {
	proto.Message
	GetName() string
}

// mergeOverlay merges overlay into base in place with proto.Merge semantics: a set
// scalar replaces the base value, messages merge field by field, an Any or a map entry
// replaces the base value whole, and lists append. Two kinds of list are exceptions:
//
//   - static_resources.clusters, static_resources.listeners, static_resources.secrets
//     and overload_manager.resource_monitors merge by name. An overlay entry replaces
//     the base entry with the same name whole; other overlay entries are appended.
//   - layered_runtime.layers are inserted immediately before the first admin layer, so
//     values set through the admin endpoint /runtime_modify keep overriding them.
//
// The caller has already rejected an overlay runtime layer whose name is in base. The
// overlay is not modified.
func mergeOverlay(base, overlay *envoybootstrapv3.Bootstrap) {
	// Take the lists with their own rules out of a copy of the overlay.
	ov := proto.CloneOf(overlay)
	layers := ov.GetLayeredRuntime().GetLayers()
	ov.LayeredRuntime = nil
	clusters := ov.GetStaticResources().GetClusters()
	listeners := ov.GetStaticResources().GetListeners()
	secrets := ov.GetStaticResources().GetSecrets()
	if ov.StaticResources != nil {
		ov.StaticResources.Clusters = nil
		ov.StaticResources.Listeners = nil
		ov.StaticResources.Secrets = nil
	}
	monitors := ov.GetOverloadManager().GetResourceMonitors()
	if ov.OverloadManager != nil {
		ov.OverloadManager.ResourceMonitors = nil
	}

	// Merge everything else with proto.Merge.
	proto.Merge(base, ov)

	// Merge the named lists by name.
	if len(clusters)+len(listeners)+len(secrets) > 0 {
		if base.StaticResources == nil {
			base.StaticResources = &envoybootstrapv3.Bootstrap_StaticResources{}
		}
		base.StaticResources.Clusters = mergeByName(base.StaticResources.Clusters, clusters)
		base.StaticResources.Listeners = mergeByName(base.StaticResources.Listeners, listeners)
		base.StaticResources.Secrets = mergeByName(base.StaticResources.Secrets, secrets)
	}
	if len(monitors) > 0 {
		if base.OverloadManager == nil {
			base.OverloadManager = &envoyoverloadv3.OverloadManager{}
		}
		base.OverloadManager.ResourceMonitors = mergeByName(base.OverloadManager.ResourceMonitors, monitors)
	}

	// Insert the runtime layers before the first admin layer, or at the end if there is none.
	if len(layers) == 0 {
		return
	}
	if base.LayeredRuntime == nil {
		base.LayeredRuntime = &envoybootstrapv3.LayeredRuntime{}
	}
	baseLayers := base.LayeredRuntime.Layers
	insertAt := slices.IndexFunc(baseLayers, func(l *envoybootstrapv3.RuntimeLayer) bool { return l.GetAdminLayer() != nil })
	if insertAt < 0 {
		insertAt = len(baseLayers)
	}
	base.LayeredRuntime.Layers = slices.Insert(baseLayers, insertAt, layers...)
}

// mergeByName replaces each base entry that has the name of an overlay entry, and
// appends the other overlay entries in order. An entry with no name never matches,
// so Envoy reports it.
func mergeByName[T namedEntry](base, overlay []T) []T {
	out := slices.Clone(base)
	for _, entry := range overlay {
		i := -1
		if entry.GetName() != "" {
			i = slices.IndexFunc(out, func(b T) bool { return b.GetName() == entry.GetName() })
		}
		if i >= 0 {
			out[i] = entry
		} else {
			out = append(out, entry)
		}
	}
	return out
}
