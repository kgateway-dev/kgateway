package irtranslator_test

import (
	"context"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// hashBackend is the fixture the fold is evaluated over: a backend with a
// backing object carrying every field the whole-object fallback reads.
func hashBackend() *ir.BackendObjectIR {
	b := overlayBackend()
	b.Obj = &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Namespace: "ns", Name: "name", UID: "uid", ResourceVersion: "1", Generation: 1,
		Labels: map[string]string{"app": "name"},
	}}
	return b
}

// withAnnotation returns a copy of the fixture whose backing object carries one
// more annotation: a write that changes nothing any hook reads.
func withAnnotation(b *ir.BackendObjectIR, value string) *ir.BackendObjectIR {
	out := *b
	svc := b.Obj.(*corev1.Service).DeepCopy()
	svc.Annotations = map[string]string{"meta.helm.sh/release-name": value}
	svc.ResourceVersion = value
	out.Obj = svc
	return &out
}

func noopOverlay(krt.HandlerContext, context.Context, ir.UniquelyConnectedClient, ir.BackendObjectIR) *sdk.ClusterOverlay {
	return nil
}

func noopLegacy(krt.HandlerContext, context.Context, ir.UniquelyConnectedClient, ir.BackendObjectIR, *envoyclusterv3.Cluster) {
}

// TestOverlayInputsHash_ZeroWithoutAnyPerClientHook: with no plugin
// contributing a per-client cluster hook there is nothing a backend write can
// reach through an overlay, so the fold carries no information and is zero.
func TestOverlayInputsHash_ZeroWithoutAnyPerClientHook(t *testing.T) {
	bt := edsBackendTranslator(map[schema.GroupKind]sdk.PolicyPlugin{})

	assert.Zero(t, bt.OverlayInputsHash(*hashBackend()))
}

// TestOverlayInputsHash_MovesOnlyForDeclaredInputs: a plugin that declares what
// its overlay reads gets exactly that. A write to a field outside the
// declaration leaves the fold where it was, which is what lets such a write
// stop at the base re-translation instead of rerunning every client's walk.
func TestOverlayInputsHash_MovesOnlyForDeclaredInputs(t *testing.T) {
	bt := edsBackendTranslator(map[schema.GroupKind]sdk.PolicyPlugin{
		{Group: "test", Kind: "Declared"}: {
			Name:                    "declared",
			PerClientClusterOverlay: noopOverlay,
			OverlayInputsHash: func(in ir.BackendObjectIR) uint64 {
				return utils.HashString(in.CanonicalHostname)
			},
		},
	})
	backend := hashBackend()
	before := bt.OverlayInputsHash(*backend)

	assert.Equal(t, before, bt.OverlayInputsHash(*withAnnotation(backend, "2")),
		"an annotation write no overlay declared must not move the fold")

	declared := *backend
	declared.CanonicalHostname = "other.svc.cluster.local"
	assert.NotEqual(t, before, bt.OverlayInputsHash(declared),
		"a write to a declared input must move the fold")
}

// TestOverlayInputsHash_UndeclaredOverlayFallsBackToWholeObject: an overlay
// registered without a declaration is a plugin bug. The framework cannot know
// what it reads, so it assumes everything and any write to the backing object
// moves the fold. That is never stale, only expensive.
func TestOverlayInputsHash_UndeclaredOverlayFallsBackToWholeObject(t *testing.T) {
	bt := edsBackendTranslator(map[schema.GroupKind]sdk.PolicyPlugin{
		{Group: "test", Kind: "Undeclared"}: {
			Name:                    "undeclared",
			PerClientClusterOverlay: noopOverlay,
		},
	})
	backend := hashBackend()

	assert.NotEqual(t, bt.OverlayInputsHash(*backend), bt.OverlayInputsHash(*withAnnotation(backend, "2")),
		"without a declaration every write to the backing object must move the fold")
}

// TestOverlayInputsHash_LegacyHookFallsBackToWholeObject: the deprecated
// PerClientProcessBackend cannot declare what it reads — that is the knowledge
// the overlay contract exists to capture — so it gets the same whole-object
// treatment as an undeclared overlay. A plugin that has not migrated is
// therefore correct at the cost of a walk per write, and migrating is what buys
// the saving back.
func TestOverlayInputsHash_LegacyHookFallsBackToWholeObject(t *testing.T) {
	bt := edsBackendTranslator(map[schema.GroupKind]sdk.PolicyPlugin{
		{Group: "test", Kind: "Legacy"}: {
			Name:                    "legacy",
			PerClientProcessBackend: noopLegacy, //nolint:staticcheck // exercising the deprecated hook's declaration
		},
	})
	backend := hashBackend()

	assert.NotEqual(t, bt.OverlayInputsHash(*backend), bt.OverlayInputsHash(*withAnnotation(backend, "2")),
		"a legacy hook cannot declare its inputs, so every write to the backing object must move the fold")
}

// TestOverlayInputsHash_MigratedPluginDoesNotPayForItsLegacyHook: a plugin
// registering both hooks is treated as migrated everywhere else, and the fold
// must agree. Taking the legacy hook's whole-object declaration here would
// silently undo the saving the migration was for.
func TestOverlayInputsHash_MigratedPluginDoesNotPayForItsLegacyHook(t *testing.T) {
	bt := edsBackendTranslator(map[schema.GroupKind]sdk.PolicyPlugin{
		{Group: "test", Kind: "Migrated"}: {
			Name:                    "migrated",
			PerClientClusterOverlay: noopOverlay,
			OverlayInputsHash: func(in ir.BackendObjectIR) uint64 {
				return utils.HashString(in.CanonicalHostname)
			},
			PerClientProcessBackend: noopLegacy, //nolint:staticcheck // a plugin mid-migration registers both
		},
	})
	backend := hashBackend()

	assert.Equal(t, bt.OverlayInputsHash(*backend), bt.OverlayInputsHash(*withAnnotation(backend, "2")),
		"the overlay's declaration wins, so the write must not move the fold")
}

// TestOverlayInputsHash_MixesPluginIdentity: two plugins reporting each other's
// values are a different configuration and must fold differently. Without the
// plugin's identity in the fold the two would collide and a swap would be
// invisible.
func TestOverlayInputsHash_MixesPluginIdentity(t *testing.T) {
	first := schema.GroupKind{Group: "test", Kind: "First"}
	second := schema.GroupKind{Group: "test", Kind: "Second"}
	declaring := func(value uint64) sdk.PolicyPlugin {
		return sdk.PolicyPlugin{
			PerClientClusterOverlay: noopOverlay,
			OverlayInputsHash:       func(ir.BackendObjectIR) uint64 { return value },
		}
	}
	backend := hashBackend()

	straight := edsBackendTranslator(map[schema.GroupKind]sdk.PolicyPlugin{
		first: declaring(1), second: declaring(2),
	})
	swapped := edsBackendTranslator(map[schema.GroupKind]sdk.PolicyPlugin{
		first: declaring(2), second: declaring(1),
	})

	assert.NotEqual(t, straight.OverlayInputsHash(*backend), swapped.OverlayInputsHash(*backend))
}

// TestOverlayInputsHash_IsStableAcrossCalls: the fold is compared against the
// value stored on the previous base row, so an unchanged backend must produce
// an unchanged value. Plugins are held in a map, and folding in map order would
// make the value vary run to run and rerun every client for nothing.
func TestOverlayInputsHash_IsStableAcrossCalls(t *testing.T) {
	policies := map[schema.GroupKind]sdk.PolicyPlugin{}
	for _, kind := range []string{"A", "B", "C", "D", "E", "F", "G", "H"} {
		policies[schema.GroupKind{Group: "test", Kind: kind}] = sdk.PolicyPlugin{
			PerClientClusterOverlay: noopOverlay,
			OverlayInputsHash:       func(in ir.BackendObjectIR) uint64 { return utils.HashString(in.CanonicalHostname) },
		}
	}
	backend := hashBackend()

	// A fresh translator per call: the ordering must come from the sort, not
	// from a map iteration that happened to be cached by the first call.
	want := edsBackendTranslator(policies).OverlayInputsHash(*backend)
	for range 20 {
		assert.Equal(t, want, edsBackendTranslator(policies).OverlayInputsHash(*backend))
	}
}
