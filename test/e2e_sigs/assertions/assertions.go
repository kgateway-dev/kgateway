//go:build e2e

package assertions

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/onsi/gomega"
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
	assertHTTPResponse200(t, gatewayAddress, port)
}

// assertHTTPResponse200 validates that the gateway responds with HTTP 200 on the given port.
// Retries for up to 30s using Gomega's Eventually.
func assertHTTPResponse200(t *testing.T, address string, port int) {
	t.Helper()
	assertHTTPResponse(t, address, port, http.StatusOK)
}

// assertHTTPResponse sends an HTTP request to the gateway on the given port
// and validates the expected response code. Retries for up to 30s using Gomega's Eventually.
func assertHTTPResponse(t *testing.T, address string, port int, expectedStatus int) {
	t.Helper()

	url := fmt.Sprintf("http://%s:%d", address, port)

	gomega.Eventually(func() error {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("failed to build request: %w", err)
		}
		req.Host = "example.com"

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("port %d: request failed: %w", port, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != expectedStatus {
			return fmt.Errorf("port %d: expected status %d, got %d", port, expectedStatus, resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("port %d: failed to read response body: %w", port, err)
		}

		if !strings.Contains(string(body), echoResponseMarker) {
			return fmt.Errorf("port %d: response does not contain %q", port, echoResponseMarker)
		}

		return nil
	}, "30s", "1s").Should(gomega.Succeed(), "gateway should respond with status %d on port %d", expectedStatus, port)
}
