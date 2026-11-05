package envoyinit

import (
	"os"
	"path/filepath"
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/protoutils"
)

func TestMergeBootstrapOverlay(t *testing.T) {
	tests := []struct {
		name       string
		base       string
		overlay    string
		wantLayers []string
		wantErr    string
		check      func(t *testing.T, merged *envoybootstrapv3.Bootstrap)
	}{
		{
			name: "overlay runtime layers are inserted before the admin layer",
			base: `
layered_runtime:
  layers:
  - {name: static_layer, static_layer: {a: 1}}
  - {name: admin_layer, admin_layer: {}}`,
			overlay: `
layered_runtime:
  layers:
  - {name: o1, static_layer: {a: 2}}
  - {name: o2, static_layer: {b: 3}}`,
			wantLayers: []string{"static_layer", "o1", "o2", "admin_layer"},
		},
		{
			name: "overlay runtime layers are appended when base has no admin layer",
			base: `
layered_runtime:
  layers:
  - {name: static_layer, static_layer: {a: 1}}`,
			overlay: `
layered_runtime:
  layers:
  - {name: o1, static_layer: {a: 2}}`,
			wantLayers: []string{"static_layer", "o1"},
		},
		{
			name: "overlay runtime layers become the runtime when base has none",
			base: `node: {id: node1}`,
			overlay: `
layered_runtime:
  layers:
  - {name: o1, static_layer: {a: 2}}`,
			wantLayers: []string{"o1"},
		},
		{
			name: "overlay without runtime leaves base layers untouched",
			base: `
layered_runtime:
  layers:
  - {name: static_layer, static_layer: {a: 1}}
  - {name: admin_layer, admin_layer: {}}`,
			overlay:    `stats_flush_interval: 10s`,
			wantLayers: []string{"static_layer", "admin_layer"},
		},
		{
			name: "duplicate layer name is rejected",
			base: `
layered_runtime:
  layers:
  - {name: static_layer, static_layer: {a: 1}}`,
			overlay: `
layered_runtime:
  layers:
  - {name: static_layer, static_layer: {a: 2}}`,
			wantErr: `overlay runtime layer "static_layer" duplicates a layer name in the bootstrap`,
		},
		{
			name: "overlay scalars win, messages merge and lists append",
			base: `
admin: {address: {socket_address: {address: 127.0.0.1, port_value: 19000}}}
node: {id: base-id, cluster: base-cluster}
static_resources:
  clusters:
  - {name: base_cluster}`,
			overlay: `
admin: {address: {socket_address: {port_value: 19001}}}
node: {cluster: overlay-cluster}
static_resources:
  clusters:
  - {name: overlay_cluster}`,
			check: func(t *testing.T, merged *envoybootstrapv3.Bootstrap) {
				sa := merged.GetAdmin().GetAddress().GetSocketAddress()
				assert.Equal(t, "127.0.0.1", sa.GetAddress(), "unset overlay field keeps the base value")
				assert.Equal(t, uint32(19001), sa.GetPortValue(), "set overlay scalar replaces the base value")
				assert.Equal(t, "base-id", merged.GetNode().GetId())
				assert.Equal(t, "overlay-cluster", merged.GetNode().GetCluster())
				var names []string
				for _, c := range merged.GetStaticResources().GetClusters() {
					names = append(names, c.GetName())
				}
				assert.Equal(t, []string{"base_cluster", "overlay_cluster"}, names)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var base, overlay envoybootstrapv3.Bootstrap
			require.NoError(t, protoutils.UnmarshalYaml([]byte(tt.base), &base))
			require.NoError(t, protoutils.UnmarshalYaml([]byte(tt.overlay), &overlay))
			overlayBefore := overlay.String()

			err := mergeBootstrapOverlay(&base, &overlay)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, overlayBefore, overlay.String(), "overlay must not be mutated")
			if tt.wantLayers != nil {
				var names []string
				for _, l := range base.GetLayeredRuntime().GetLayers() {
					names = append(names, l.GetName())
				}
				assert.Equal(t, tt.wantLayers, names)
			}
			if tt.check != nil {
				tt.check(t, &base)
			}
		})
	}
}

func TestApplyOverlayFile(t *testing.T) {
	dir := t.TempDir()
	overlayPath := filepath.Join(dir, "overlay.yaml")

	t.Run("missing file", func(t *testing.T) {
		_, err := applyOverlayFile(`node: {id: node1}`, filepath.Join(dir, "missing.yaml"))
		require.ErrorContains(t, err, "failed to read bootstrap overlay")
	})

	t.Run("unknown field is rejected", func(t *testing.T) {
		require.NoError(t, os.WriteFile(overlayPath, []byte(`layerd_runtime: {}`), 0o600))
		_, err := applyOverlayFile(`node: {id: node1}`, overlayPath)
		require.ErrorContains(t, err, "failed to unmarshal bootstrap overlay")
	})

	t.Run("merged output is JSON Envoy can load", func(t *testing.T) {
		require.NoError(t, os.WriteFile(overlayPath, []byte(`node: {cluster: c}`), 0o600))
		merged, err := applyOverlayFile(`node: {id: node1}`, overlayPath)
		require.NoError(t, err)
		assert.JSONEq(t, `{"node":{"id":"node1","cluster":"c"}}`, merged)
	})
}
