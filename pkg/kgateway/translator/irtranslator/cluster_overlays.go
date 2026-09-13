package irtranslator

import (
	"cmp"
	"hash/fnv"
	"slices"
	"strconv"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// ClusterOverlayPlugin is a plugin's per-client cluster overlay together with
// the declaration of what it reads from the backend and its stable identity.
// Construct through OrderedClusterOverlays.
type ClusterOverlayPlugin struct {
	groupKind  schema.GroupKind
	name       string
	overlay    sdk.PerClientClusterOverlay
	inputsHash sdk.OverlayInputsHash
}

// OrderedClusterOverlays collects every contributed PerClientClusterOverlay in
// (Group, Kind, Name) order. ContributedPolicies is a map, and both the order
// overlays mutate a clone in and the fold of their declared inputs must be
// byte-stable across recomputes, so the order is fixed here once.
//
// An overlay registered without an OverlayInputsHash is a plugin bug: the
// framework cannot know what it reads, so it is logged and given a
// declaration that treats every write to the backing object as a change. That
// is never stale, only expensive; see sdk.OverlayInputsHash.
func OrderedClusterOverlays(policies sdk.ContributesPolicies) []ClusterOverlayPlugin {
	overlays := make([]ClusterOverlayPlugin, 0, len(policies))
	for groupKind, policyPlugin := range policies {
		if policyPlugin.PerClientClusterOverlay == nil {
			continue
		}
		inputsHash := policyPlugin.OverlayInputsHash
		if inputsHash == nil {
			logger.Error("per-client cluster overlay registered without OverlayInputsHash; every write to a backend will rerun every client for it",
				"group", groupKind.Group, "kind", groupKind.Kind, "plugin", policyPlugin.Name)
			inputsHash = wholeObjectInputsHash
		}
		overlays = append(overlays, ClusterOverlayPlugin{
			groupKind:  groupKind,
			name:       policyPlugin.Name,
			overlay:    policyPlugin.PerClientClusterOverlay,
			inputsHash: inputsHash,
		})
	}
	slices.SortStableFunc(overlays, func(a, b ClusterOverlayPlugin) int {
		if c := cmp.Compare(a.groupKind.Group, b.groupKind.Group); c != 0 {
			return c
		}
		if c := cmp.Compare(a.groupKind.Kind, b.groupKind.Kind); c != 0 {
			return c
		}
		return cmp.Compare(a.name, b.name)
	})
	return overlays
}

// wholeObjectInputsHash is the fallback declaration for an overlay that made
// none: the backing object's identity and every version field it has, so any
// write to it counts as a change. IR fields derived from the object move with
// it; the base proto hash covers the rest.
func wholeObjectInputsHash(backend ir.BackendObjectIR) uint64 {
	if backend.Obj == nil {
		return 0
	}
	hasher := fnv.New64a()
	utils.HashStringField(hasher, string(backend.Obj.GetUID()))
	utils.HashStringField(hasher, backend.Obj.GetResourceVersion())
	utils.HashStringField(hasher, strconv.FormatInt(backend.Obj.GetGeneration(), 10))
	return hasher.Sum64()
}

func (t *BackendTranslator) orderedClusterOverlays() []ClusterOverlayPlugin {
	if t.ClusterOverlays != nil {
		return t.ClusterOverlays
	}
	return OrderedClusterOverlays(t.ContributedPolicies)
}

// OverlayInputsHash folds every overlay plugin's declared backend inputs into
// one value, in plugin order and mixed with each plugin's identity, so two
// plugins reporting swapped values do not collide. It is zero when no plugin
// contributes an overlay. The base row carries it alongside the cluster
// proto's hash: together they say whether any client's view of this backend
// can have changed, which is what lets a write that moves neither stop at the
// base re-translation.
func (t *BackendTranslator) OverlayInputsHash(backend ir.BackendObjectIR) uint64 {
	overlays := t.orderedClusterOverlays()
	if len(overlays) == 0 {
		return 0
	}
	hasher := fnv.New64a()
	for _, overlay := range overlays {
		utils.HashStringField(hasher, overlay.groupKind.Group)
		utils.HashStringField(hasher, overlay.groupKind.Kind)
		utils.HashStringField(hasher, overlay.name)
		utils.HashUint64(hasher, overlay.inputsHash(backend))
	}
	return hasher.Sum64()
}
