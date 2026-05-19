//go:build e2e

package common

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"istio.io/istio/pkg/test/util/retry"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

// Gateway is a curl-able handle for a Gateway resource. Address is the literal value used by
// curl — callers populate it however they like (LB IP, in-cluster DNS, port-forward, etc.).
// GATEWAY_ADDRESS_OVERRIDE is applied only by SetupBaseGateway; direct constructions of this type
// do not consult it.
type Gateway struct {
	types.NamespacedName
	Address string
}

// Defaults for SendConsistently — mirror assertions.AssertEventuallyConsistentCurlResponse.
const (
	defaultConsistencyWindow = 3 * time.Second
	defaultConsistencyPoll   = 1 * time.Second
)

// Send curls the gateway and waits until the response matches.
func (g *Gateway) Send(t *testing.T, match *matchers.HttpResponse, opts ...curl.Option) {
	t.Helper()
	g.SendWithRetry(t, match, nil, opts...)
}

// SendWithRetry curls the gateway with caller-supplied retry options
// (e.g. retry.Timeout, retry.Delay) for controlling the eventual-match behavior.
func (g *Gateway) SendWithRetry(t *testing.T, match *matchers.HttpResponse, retryOpts []retry.Option, opts ...curl.Option) {
	t.Helper()
	fullOpts := g.curlOpts(opts)
	retry.UntilSuccessOrFail(t, func() error {
		return g.matchOnce(fullOpts, match)
	}, retryOpts...)
}

// SendConsistently curls the gateway, waits for the response to eventually match,
// then asserts the response continues to match over a 3s window polled every 1s.
// Mirrors assertions.AssertEventuallyConsistentCurlResponse semantics.
func (g *Gateway) SendConsistently(t *testing.T, match *matchers.HttpResponse, opts ...curl.Option) {
	t.Helper()
	g.SendConsistentlyFor(t, match, defaultConsistencyWindow, defaultConsistencyPoll, opts...)
}

// SendConsistentlyFor is SendConsistently with caller-supplied window and polling interval.
// Any divergence within the window fails the test (no per-iteration retries).
func (g *Gateway) SendConsistentlyFor(t *testing.T, match *matchers.HttpResponse, window, poll time.Duration, opts ...curl.Option) {
	t.Helper()
	if poll <= 0 {
		t.Fatalf("SendConsistentlyFor: poll interval must be positive, got %v", poll)
	}
	g.Send(t, match, opts...)

	fullOpts := g.curlOpts(opts)
	windowTimer := time.NewTimer(window)
	defer windowTimer.Stop()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	for {
		select {
		case <-windowTimer.C:
			return
		case <-ticker.C:
			if err := g.matchOnce(fullOpts, match); err != nil {
				t.Fatalf("response did not consistently match within %v: %v", window, err)
			}
		}
	}
}

// matchOnce executes a single curl and returns an error if the response does not match.
// The body is buffered before matching so it can be included in failure diagnostics
// (the underlying gomega HaveHttpResponse matcher does not print the actual body —
// see test/gomega/matchers/have_http_response.go).
func (g *Gateway) matchOnce(fullOpts []curl.Option, match *matchers.HttpResponse) error {
	r, err := curl.ExecuteRequest(fullOpts...)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	mm := matchers.HaveHttpResponse(match)
	success, matchErr := mm.Match(r)
	if matchErr != nil {
		return matchErr
	}
	if !success {
		return fmt.Errorf("%s\nactual: status=%d body=%q", mm.FailureMessage(r), r.StatusCode, string(body))
	}
	return nil
}

func (g *Gateway) curlOpts(opts []curl.Option) []curl.Option {
	var hostOpt curl.Option
	if _, _, err := net.SplitHostPort(g.Address); err == nil {
		hostOpt = curl.WithHostPort(g.Address)
	} else {
		hostOpt = curl.WithHost(g.Address)
	}
	return append([]curl.Option{hostOpt}, opts...)
}
