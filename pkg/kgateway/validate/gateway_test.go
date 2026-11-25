package validate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestReservedPorts(t *testing.T) {
	for name, tc := range map[string]struct {
		vb   *ir.GatewayValidationBootstraps
		want []uint32
	}{
		"a Gateway without validation bootstraps reserves the default ports": {
			want: DefaultReservedPorts(),
		},
		"validation bootstraps without ports reserve the default ports": {
			vb:   &ir.GatewayValidationBootstraps{},
			want: DefaultReservedPorts(),
		},
		"the current bootstrap's ports are added to the default ports, sorted and without duplicates": {
			vb:   &ir.GatewayValidationBootstraps{ReservedPorts: []uint32{8443, 8083, 9091, 19000}},
			want: []uint32{8082, 8083, 8443, 9091, 19000},
		},
	} {
		t.Run(name, func(t *testing.T) {
			// Check the reserved ports.
			assert.Equal(t, tc.want, ReservedPorts(tc.vb))

			// Every reserved port is rejected, and no other port is.
			for _, p := range tc.want {
				require.ErrorIs(t, ListenerPortForBootstrap(gwv1.PortNumber(p), tc.vb), ErrListenerPortReserved, "port %d", p) //nolint:gosec // G115: test ports are small
			}
			assert.NoError(t, ListenerPortForBootstrap(8080, tc.vb))
		})
	}
}

func TestListenerPortForBootstrapMessages(t *testing.T) {
	// A default port is reserved without naming a bootstrap.
	assert.EqualError(t, ListenerPortForBootstrap(8082, nil), "invalid port 8082 in listener: port is reserved")

	// A port only the merged bootstrap listens on names the overlay.
	vb := &ir.GatewayValidationBootstraps{ReservedPorts: []uint32{8083, 9091, 19000}}
	assert.EqualError(t, ListenerPortForBootstrap(8083, vb),
		"invalid port 8083 in listener: the proxy bootstrap, with the GatewayParameters bootstrap overlay applied, listens on it: port is reserved")

	// A default port stays reserved after the overlay moves its listener off it.
	assert.EqualError(t, ListenerPortForBootstrap(8082, vb), "invalid port 8082 in listener: port is reserved")
}
