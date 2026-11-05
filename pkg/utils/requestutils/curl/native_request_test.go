package curl

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

// WithConnectionTimeout must still let the caller read a body larger than one read chunk after
// ExecuteRequest returns.
func TestExecuteRequestWithConnectionTimeoutReadsWholeBody(t *testing.T) {
	body := strings.Repeat("x", 100_000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	host, port, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	portNum, err := strconv.Atoi(port)
	require.NoError(t, err)

	resp, err := ExecuteRequest(WithHost(host), WithPort(portNum), WithConnectionTimeout(5))
	require.NoError(t, err)
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading the body should not be cut off by the timeout")
	assert.Len(t, got, len(body))
}

// WithFormBody sends the body with a form Content-Type, as `curl -d` does.
func TestExecuteRequestWithFormBody(t *testing.T) {
	var gotContentType, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
	}))
	defer server.Close()

	host, port, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	portNum, err := strconv.Atoi(port)
	require.NoError(t, err)

	resp, err := ExecuteRequest(WithHost(host), WithPort(portNum), WithFormBody("hello"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "application/x-www-form-urlencoded", gotContentType)
	assert.Equal(t, "hello", gotBody)
}
