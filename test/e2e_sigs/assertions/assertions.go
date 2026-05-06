//go:build e2e

package assertions

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	// echoResponseMarker is a substring the gateway-api echo-basic server
	// includes in every response body (it echoes back the pod name in JSON).
	echoResponseMarker = "echo-server"
)

// AssertSuccessfulResponse validates that the gateway address responds with HTTP 200
// on the given port and includes the echo response body.
func AssertSuccessfulResponse(t *testing.T, gatewayAddress string, port int) {
	t.Helper()
	assertHTTPResponse(t, gatewayAddress, port, http.StatusOK)
}

// assertHTTPResponse sends an HTTP request to the gateway on the given port
// and validates the expected response code. Retries for up to 30s.
func assertHTTPResponse(t *testing.T, address string, port int, expectedStatus int) {
	t.Helper()

	url := fmt.Sprintf("http://%s:%d", address, port)

	assert.Eventually(t, func() bool {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return false
		}
		req.Host = "example.com"

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()

		if resp.StatusCode != expectedStatus {
			return false
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return false
		}

		// TODO: add optional body matchers
		if !strings.Contains(string(body), echoResponseMarker) {
			return false
		}

		return true
	}, 30*time.Second, 1*time.Second, "gateway should respond with status %d on port %d", expectedStatus, port)
}
