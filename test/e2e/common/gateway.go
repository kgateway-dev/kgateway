//go:build e2e

package common

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	avastretry "github.com/avast/retry-go/v4"
	"istio.io/istio/pkg/test/util/retry"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/portforward"
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

	// forwards, when set, sends requests through port-forwards to the Gateway's Service instead
	// of Address. Set by NewGateway under GATEWAY_PORT_FORWARD.
	forwards *portForwards
}

// Defaults for SendEventuallyConsistent — mirror values in assertions.AssertEventuallyConsistentCurlResponse.
const (
	defaultConsistencyWindow = 3 * time.Second
	defaultConsistencyPoll   = 1 * time.Second
)

// Send curls the gateway and waits until the response matches using Istio default retry options (10s timeout, 100ms delay).
func (g *Gateway) Send(t *testing.T, match *matchers.HttpResponse, opts ...curl.Option) {
	t.Helper()
	// Pass t.Context to match the signature
	g.SendWithRetry(t.Context(), t, match, nil, opts...)
}

// SendWithRetry curls the gateway with caller-supplied retry options
// (e.g. retry.Timeout, retry.Delay) for controlling the eventual-match behavior.
func (g *Gateway) SendWithRetry(ctx context.Context, t *testing.T, match *matchers.HttpResponse, retryOpts []retry.Option, opts ...curl.Option) {
	t.Helper()
	fullOpts := g.curlOpts(opts)
	retry.UntilSuccessOrFail(t, func() error {
		if err := ctx.Err(); err != nil {
			t.Fatalf("send retry interrupted by ctx: %v", err)
		}
		return g.matchOnce(ctx, fullOpts, match)
	}, retryOpts...)
}

// SendOnce curls the gateway once and returns an error if the response does not match,
// for callers that poll inside their own Eventually loop.
func (g *Gateway) SendOnce(ctx context.Context, match *matchers.HttpResponse, opts ...curl.Option) error {
	return g.matchOnce(ctx, g.curlOpts(opts), match)
}

// SendEventuallyConsistent curls the gateway, waits for the response to eventually match,
// then asserts the response continues to match over a 3s window polled every 1s.
// Mirrors assertions.AssertEventuallyConsistentCurlResponse semantics.
func (g *Gateway) SendEventuallyConsistent(ctx context.Context, t *testing.T, match *matchers.HttpResponse, opts ...curl.Option) {
	t.Helper()
	g.SendConsistentlyFor(ctx, t, match, defaultConsistencyWindow, defaultConsistencyPoll, opts...)
}

// SendConsistentlyFor is SendEventuallyConsistent with caller-supplied window and polling interval.
// Both phases use window/poll: the eventual-wait phase as its retry timeout/delay, and the
// consistency phase as its window/poll.
func (g *Gateway) SendConsistentlyFor(ctx context.Context, t *testing.T, match *matchers.HttpResponse, window, poll time.Duration, opts ...curl.Option) {
	t.Helper()
	if poll <= 0 {
		t.Fatalf("SendConsistentlyFor: poll interval must be positive, got %v", poll)
	}
	if window <= 0 {
		t.Fatalf("SendConsistentlyFor: consistency window must be positive, got %v", window)
	}
	if window <= poll {
		t.Fatalf("SendConsistentlyFor: consistency window (%v) must be greater than poll interval (%v)", window, poll)
	}

	// Get the initial match before entering the consistency loop
	g.SendWithRetry(ctx, t, match, []retry.Option{retry.Timeout(window), retry.Delay(poll)}, opts...)

	fullOpts := g.curlOpts(opts)
	windowTimer := time.NewTimer(window)
	defer windowTimer.Stop()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("consistency check interrupted by ctx: %v", ctx.Err())
		case <-windowTimer.C:
			return
		case <-ticker.C:
			if err := g.matchOnce(ctx, fullOpts, match); err != nil {
				t.Fatalf("response did not consistently match within %v: %v", window, err)
			}
		}
	}
}

// matchOnce executes a single curl and returns an error if the response does not match.
// The body is buffered before matching so it can be included in failure diagnostics
// (the underlying gomega HaveHttpResponse matcher does not print the actual body —
// see test/gomega/matchers/have_http_response.go).
func (g *Gateway) matchOnce(ctx context.Context, fullOpts []curl.Option, match *matchers.HttpResponse) error {
	r, err := curl.ExecuteRequest(curl.Extend(fullOpts, curl.WithContext(ctx))...)
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
	base := []curl.Option{hostOpt}
	if g.forwards != nil {
		base = append(base, curl.WithDialContext(g.forwards.dial))
	}
	return curl.Extend(base, opts...)
}

// portForwards opens one port-forward per destination port of a Gateway's Service on first use.
// The Service shares the Gateway's name and maps each listener port to the same container port.
type portForwards struct {
	kubeContext string
	service     types.NamespacedName

	mu     sync.Mutex
	byPort map[string]portforward.PortForwarder
}

func newPortForwards(kubeContext string, service types.NamespacedName) *portForwards {
	return &portForwards{
		kubeContext: kubeContext,
		service:     service,
		byPort:      map[string]portforward.PortForwarder{},
	}
}

// dial ignores the host in addr and connects to the port-forward for its port.
func (p *portForwards) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	fwd, err := p.get(ctx, port)
	if err != nil {
		return nil, err
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, network, fwd.Address())
	if err != nil {
		// The forward may point at a pod that is gone; reopen it on the next attempt.
		p.drop(port, fwd)
	}
	return conn, err
}

func (p *portForwards) get(ctx context.Context, port string) (portforward.PortForwarder, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if fwd, ok := p.byPort[port]; ok {
		return fwd, nil
	}
	remotePort, err := strconv.Atoi(port)
	if err != nil {
		return nil, err
	}
	fwd := portforward.NewApiPortForwarder(
		portforward.WithKubeContext(p.kubeContext),
		portforward.WithService(p.service.Name, p.service.Namespace),
		portforward.WithRemotePort(remotePort),
	)
	// One attempt: the caller's retry loop retries the whole request.
	if err := fwd.Start(ctx, avastretry.Attempts(1)); err != nil {
		return nil, fmt.Errorf("port-forward to service %s port %d: %w", p.service, remotePort, err)
	}
	p.byPort[port] = fwd
	return fwd, nil
}

func (p *portForwards) drop(port string, fwd portforward.PortForwarder) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.byPort[port] == fwd {
		delete(p.byPort, port)
		fwd.Close()
	}
}

func (p *portForwards) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for port, fwd := range p.byPort {
		fwd.Close()
		delete(p.byPort, port)
	}
}
