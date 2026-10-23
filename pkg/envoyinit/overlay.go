package envoyinit

import (
	"fmt"
	"os"
	"slices"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/protoutils"
)

// applyOverlayFile merges the partial bootstrap in overlayPath over bootstrapConfig and
// returns the merged bootstrap as JSON.
func applyOverlayFile(bootstrapConfig, overlayPath string) (string, error) {
	overlayConfig, err := os.ReadFile(overlayPath) //nolint:gosec // G304: path comes from the pod spec, like the input config
	if err != nil {
		return "", fmt.Errorf("failed to read bootstrap overlay: %w", err)
	}
	var base, overlay envoybootstrapv3.Bootstrap
	if err := protoutils.UnmarshalYaml([]byte(bootstrapConfig), &base); err != nil {
		return "", fmt.Errorf("failed to unmarshal bootstrap config: %w", err)
	}
	if err := protoutils.UnmarshalYaml(overlayConfig, &overlay); err != nil {
		return "", fmt.Errorf("failed to unmarshal bootstrap overlay %s: %w", overlayPath, err)
	}
	if err := mergeBootstrapOverlay(&base, &overlay); err != nil {
		return "", fmt.Errorf("failed to merge bootstrap overlay %s: %w", overlayPath, err)
	}
	merged, err := protoutils.MarshalBytes(&base)
	if err != nil {
		return "", fmt.Errorf("failed to marshal merged bootstrap config: %w", err)
	}
	return string(merged), nil
}

// mergeBootstrapOverlay merges overlay into base with proto.Merge semantics: set scalar
// fields in overlay replace those in base, messages merge recursively, and repeated
// fields are appended. Because the overlay is a proto, it cannot reset a base field to
// its zero value.
//
// Runtime layers are the exception to plain appending. Envoy resolves a key from the last
// layer that sets it, and the admin layer must stay last so /runtime_modify keeps
// winning, so overlay layers are inserted immediately before the first admin layer in
// base (or appended if base has none). An overlay layer whose name is already used in
// base is an error.
func mergeBootstrapOverlay(base, overlay *envoybootstrapv3.Bootstrap) error {
	overlayLayers := overlay.GetLayeredRuntime().GetLayers()
	if len(overlayLayers) > 0 {
		overlay = proto.CloneOf(overlay)
		overlay.LayeredRuntime = nil
	}
	proto.Merge(base, overlay)
	if len(overlayLayers) == 0 {
		return nil
	}

	if base.GetLayeredRuntime() == nil {
		base.LayeredRuntime = &envoybootstrapv3.LayeredRuntime{}
	}
	baseLayers := base.GetLayeredRuntime().GetLayers()
	for _, l := range overlayLayers {
		if slices.ContainsFunc(baseLayers, func(b *envoybootstrapv3.RuntimeLayer) bool { return b.GetName() == l.GetName() }) {
			return fmt.Errorf("overlay runtime layer %q duplicates a layer name in the bootstrap", l.GetName())
		}
	}
	insertAt := slices.IndexFunc(baseLayers, func(l *envoybootstrapv3.RuntimeLayer) bool { return l.GetAdminLayer() != nil })
	if insertAt < 0 {
		insertAt = len(baseLayers)
	}
	base.LayeredRuntime.Layers = slices.Insert(baseLayers, insertAt, overlayLayers...)
	return nil
}
