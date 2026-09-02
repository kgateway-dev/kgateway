package setup

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The default (empty) bind address must accept clients of both families.
func TestNewXDSListenerDefaultsToDualStack(t *testing.T) {
	l, err := newXDSListener("", 0)
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })
	serveAndDiscard(t, l)

	addr, ok := l.Addr().(*net.TCPAddr)
	require.True(t, ok, "expected a TCP listener")
	assert.True(t, addr.IP.IsUnspecified(), "default bind should be the unspecified address, got %s", addr.IP)

	assert.True(t, canDial(t, "tcp4", "127.0.0.1", l), "an IPv4 client should reach the default bind")
	assert.True(t, canDial(t, "tcp6", "::1", l), "an IPv6 client should reach the default bind")
}

// An explicit address restricts the server to that address's family. Go treats
// every wildcard on the "tcp" network as dual-stack, so this only holds because
// newXDSListener picks tcp4/tcp6 explicitly - asserting the bind address alone
// would pass even without that.
func TestNewXDSListenerExplicitAddressRestrictsFamily(t *testing.T) {
	t.Run("ipv4 wildcard refuses ipv6", func(t *testing.T) {
		l, err := newXDSListener("0.0.0.0", 0)
		require.NoError(t, err)
		t.Cleanup(func() { l.Close() })
		serveAndDiscard(t, l)

		assert.True(t, canDial(t, "tcp4", "127.0.0.1", l), "an IPv4 client should reach an IPv4 bind")
		assert.False(t, canDial(t, "tcp6", "::1", l), "an IPv6 client must not reach an IPv4-only bind")
	})

	t.Run("ipv6 wildcard refuses ipv4", func(t *testing.T) {
		l, err := newXDSListener("::", 0)
		require.NoError(t, err)
		t.Cleanup(func() { l.Close() })
		serveAndDiscard(t, l)

		assert.True(t, canDial(t, "tcp6", "::1", l), "an IPv6 client should reach an IPv6 bind")
		assert.False(t, canDial(t, "tcp4", "127.0.0.1", l), "an IPv4 client must not reach an IPv6-only bind")
	})

	t.Run("loopback pins the address", func(t *testing.T) {
		l, err := newXDSListener("127.0.0.1", 0)
		require.NoError(t, err)
		t.Cleanup(func() { l.Close() })

		addr, ok := l.Addr().(*net.TCPAddr)
		require.True(t, ok, "expected a TCP listener")
		assert.Equal(t, "127.0.0.1", addr.IP.String())
	})
}

func TestNewXDSListenerRejectsNonIP(t *testing.T) {
	_, err := newXDSListener("localhost", 0)
	assert.ErrorContains(t, err, `xds bind address "localhost" is not a valid IP address`)
}

// serveAndDiscard accepts and immediately closes connections, so a dial that
// reaches the listener completes rather than sitting in the accept backlog.
func serveAndDiscard(t *testing.T, l net.Listener) {
	t.Helper()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
}

func canDial(t *testing.T, network, host string, l net.Listener) bool {
	t.Helper()
	_, port, err := net.SplitHostPort(l.Addr().String())
	require.NoError(t, err)

	conn, err := net.Dial(network, net.JoinHostPort(host, port))
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
