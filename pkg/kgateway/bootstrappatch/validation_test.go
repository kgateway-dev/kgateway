package bootstrappatch

import (
	"strings"
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// subsetSource is a bootstrap with every kind of runtime layer.
const subsetSource = `
layered_runtime:
  layers:
  - {name: admin_layer, admin_layer: {}}
  - name: limits
    static_layer: {re2.max_program_size.error_level: 1000, b: "x<y"}
  - {name: rtds, rtds_layer: {name: rtds, rtds_config: {ads: {}}}}
  - {name: disk, disk_layer: {symlink_root: /srv/runtime}}
  - name: overrides
    static_layer: {a: 1}
static_resources:
  listeners:
  - name: extra
    address: {socket_address: {address: 0.0.0.0, port_value: 9000}}
`

func parseYAMLBootstrap(t *testing.T, y string) *envoybootstrapv3.Bootstrap {
	t.Helper()
	bs, err := ParseBootstrap([]byte(y))
	require.NoError(t, err)
	return bs
}

func TestValidationSubset(t *testing.T) {
	// Take the subset of a bootstrap with every kind of runtime layer.
	subset := ValidationSubset(parseYAMLBootstrap(t, subsetSource))

	// Only the static layers remain, in order.
	var layers []string
	for _, l := range subset.GetLayeredRuntime().GetLayers() {
		layers = append(layers, l.GetName())
	}
	assert.Equal(t, []string{"limits", "overrides"}, layers, "only static layers belong in the subset")
	assert.Nil(t, subset.GetStaticResources(), "listeners do not belong in the subset")

	// A bootstrap without a static layer has an empty subset.
	empty, err := EncodeValidationSubset(ValidationSubset(parseYAMLBootstrap(t, `{layered_runtime: {layers: [{name: admin_layer, admin_layer: {}}]}}`)))
	require.NoError(t, err)
	assert.Equal(t, EmptyValidationSubset, empty)
}

func TestEncodeValidationSubset(t *testing.T) {
	// Encode the subset.
	subset := ValidationSubset(parseYAMLBootstrap(t, subsetSource))
	encoded, err := EncodeValidationSubset(subset)
	require.NoError(t, err)

	// Keys are sorted and snake_case, with no whitespace.
	assert.JSONEq(t, `{
		"layered_runtime": {"layers": [
			{"name": "limits", "static_layer": {"b": "x<y", "re2.max_program_size.error_level": 1000}},
			{"name": "overrides", "static_layer": {"a": 1}}
		]}
	}`, encoded)
	assert.NotContains(t, encoded, " ", "the encoding should have no whitespace")
	assert.Less(t, strings.Index(encoded, `"b"`), strings.Index(encoded, `"re2.`), "keys should be sorted")

	// The same subset, built separately, encodes to the same string.
	again, err := EncodeValidationSubset(ValidationSubset(parseYAMLBootstrap(t, subsetSource)))
	require.NoError(t, err)
	assert.Equal(t, encoded, again)

	// The encoding parses back to the subset.
	parsed, err := ParseValidationSubset(encoded)
	require.NoError(t, err)
	assert.True(t, proto.Equal(subset, parsed), "the subset should round-trip")
}

func TestValidationData(t *testing.T) {
	// Encode a subset with ports out of order and repeated.
	subset, err := EncodeValidationSubset(ValidationSubset(parseYAMLBootstrap(t, subsetSource)))
	require.NoError(t, err)
	encoded, err := EncodeValidationData(subset, []uint32{19000, 9000, 8082, 9000})
	require.NoError(t, err)

	// Decoding gives back the subset byte for byte, so the translator can compare it
	// with a pod annotation, and the ports sorted and without duplicates.
	data, err := DecodeValidationData(encoded)
	require.NoError(t, err)
	assert.Equal(t, subset, string(data.Bootstrap))
	assert.Equal(t, []uint32{8082, 9000, 19000}, data.ReservedPorts)

	// Data that is not JSON fails to decode.
	_, err = DecodeValidationData("not json")
	require.ErrorContains(t, err, ValidationKey+" does not decode")
}
