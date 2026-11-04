package curl

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// WithDialContext routes the connection through the given dialer, regardless of the URL's host.
func TestExecuteRequestWithDialContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	var dialed string
	resp, err := ExecuteRequest(
		WithHost("unreachable.invalid"),
		WithPort(8080),
		WithDialContext(func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialed = addr
			var d net.Dialer
			return d.DialContext(ctx, network, server.Listener.Addr().String())
		}),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusTeapot, resp.StatusCode)
	assert.Equal(t, "unreachable.invalid:8080", dialed, "dialer should get the URL's host and port")
}
