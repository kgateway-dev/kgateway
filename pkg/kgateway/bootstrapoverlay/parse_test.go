package bootstrapoverlay

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
)

func TestListenerPorts(t *testing.T) {
	// Parse a bootstrap with the admin address, static listeners, an additional
	// address, a pipe listener, an internal listener and UDP addresses.
	bs, err := ParseBootstrap([]byte(`
admin: {address: {socket_address: {address: 127.0.0.1, port_value: 19000}}}
static_resources:
  listeners:
  - name: a
    address: {socket_address: {address: 0.0.0.0, port_value: 8082}}
    additional_addresses:
    - address: {socket_address: {address: "::", port_value: 8443}}
  - name: b
    address: {socket_address: {address: 0.0.0.0, port_value: 8082}}
  - name: pipe
    address: {pipe: {path: /tmp/sock}}
  - name: internal
    address: {envoy_internal_address: {server_listener_name: x}}
  - name: udp
    address: {socket_address: {protocol: UDP, address: 0.0.0.0, port_value: 5353}}
    additional_addresses:
    - address: {socket_address: {protocol: UDP, address: "::", port_value: 8443}}
    - address: {socket_address: {protocol: UDP, address: "::", port_value: 5354}}
`))
	require.NoError(t, err)

	// Each TCP port appears once, sorted. The pipe, internal and UDP addresses add
	// none, and UDP 8443 does not hide TCP 8443.
	assert.Equal(t, []uint32{8082, 8443, 19000}, ListenerPorts(bs))
}

func TestMarshal(t *testing.T) {
	// Marshal a merged bootstrap.
	res, err := Apply(parseBase(t), []Overlay{overlay(t, gwParams, `{statsFlushInterval: 10s}`)}, apisettings.BootstrapOverlayManagedPathsWarn)
	require.NoError(t, err)
	out, err := Marshal(res.Bootstrap)
	require.NoError(t, err)

	// Field names are snake_case and keys are sorted.
	assert.Contains(t, string(out), "stats_flush_interval: 10s")
	assert.NotContains(t, string(out), "statsFlushInterval")
	assert.Regexp(t, `(?s)^admin:.*\ncluster_manager:.*\ndynamic_resources:.*\nlayered_runtime:`, string(out))

	// The output parses back to the same bootstrap and marshals to the same bytes.
	reparsed, err := ParseBootstrap(out)
	require.NoError(t, err)
	again, err := Marshal(reparsed)
	require.NoError(t, err)
	assert.Equal(t, string(out), string(again))
}
