package setup_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/test/envtestutil"
)

func TestDiscoveryScopeEndpoint(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	t.Setenv("KGW_DISCOVERY_SCOPE_BIND_ADDRESS", address)
	t.Setenv("KGW_ADMIN_BIND_ADDRESS", "127.0.0.1")
	t.Setenv("KGW_ENABLE_EXPERIMENTAL_GATEWAY_API_FEATURES", "true")
	t.Setenv("KGW_ENABLE_ISTIO_INTEGRATION", "false")
	t.Setenv("KGW_DISCOVERY_NAMESPACE_SELECTORS", `[{"matchLabels":{"scope":"selected"}},{"matchExpressions":[{"key":"scope","operator":"In","values":["other","second"]}]}]`)
	st, err := envtestutil.BuildSettings()
	require.NoError(t, err)

	setupEnvTestAndRun(t, st, func(t *testing.T, ctx context.Context, _ *krt.DebugHandler, client istiokube.CLIClient, _ int) {
		httpClient := &http.Client{Timeout: time.Second}
		t.Cleanup(httpClient.CloseIdleConnections)
		adminURL := "http://127.0.0.1:" + strconv.Itoa(int(wellknown.KgatewayAdminPort))
		require.Eventually(t, func() bool {
			resp, err := httpClient.Get(adminURL + "/version")
			if err != nil {
				return false
			}
			defer resp.Body.Close()
			return resp.StatusCode == http.StatusOK
		}, 15*time.Second, 100*time.Millisecond, "controller admin listener did not start")

		type watchedKind struct {
			Group    string
			Version  string
			Resource string
			Kind     string
		}
		type scope struct {
			Controller      string
			Version         string
			ControllerNames []string
			Selectors       []string
			Namespaces      []string
			Kinds           []watchedKind
		}
		scopeURL := "http://" + address + "/discovery-scope"
		readScope := func() (scope, error) {
			var result scope
			resp, err := httpClient.Get(scopeURL)
			if err != nil {
				return result, err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return result, fmt.Errorf("HTTP status %d", resp.StatusCode)
			}
			if got := resp.Header.Get("Content-Type"); got != "application/json" {
				return result, fmt.Errorf("content type %q", got)
			}
			err = json.NewDecoder(resp.Body).Decode(&result)
			return result, err
		}

		var initial scope
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			var err error
			initial, err = readScope()
			assert.NoError(c, err, "configured discovery-scope listener must serve JSON")
		}, 5*time.Second, 100*time.Millisecond)
		require.Equal(t, "kgateway", initial.Controller)
		require.NotEmpty(t, initial.Version)
		require.Equal(t, []string{wellknown.DefaultGatewayControllerName}, initial.ControllerNames)
		require.Equal(t, []string{"scope=selected", "scope in (other,second)"}, initial.Selectors)
		require.Equal(t, []string{}, initial.Namespaces)
		require.Contains(t, initial.Kinds, watchedKind{"gateway.networking.k8s.io", "v1", "httproutes", "HTTPRoute"})
		require.Contains(t, initial.Kinds, watchedKind{"gateway.kgateway.dev", "v1alpha1", "trafficpolicies", "TrafficPolicy"})
		// Envtest installs the legacy TCPRoute CRD and the promoted TLSRoute CRD.
		require.Contains(t, initial.Kinds, watchedKind{"gateway.networking.k8s.io", "v1alpha2", "tcproutes", "TCPRoute"})
		require.Contains(t, initial.Kinds, watchedKind{"gateway.networking.k8s.io", "v1", "tlsroutes", "TLSRoute"})
		require.NotContains(t, initial.Kinds, watchedKind{"networking.istio.io", "v1", "serviceentries", "ServiceEntry"})

		namespaces := client.Kube().CoreV1().Namespaces()
		for name, label := range map[string]string{"watched-b": "other", "watched-a": "selected", "excluded": "ignored"} {
			_, err := namespaces.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"scope": label}},
			}, metav1.CreateOptions{})
			require.NoError(t, err)
		}
		expectNamespaces := func(want ...string) {
			t.Helper()
			require.EventuallyWithT(t, func(c *assert.CollectT) {
				got, err := readScope()
				if assert.NoError(c, err) {
					assert.True(c, slices.Equal(want, got.Namespaces), "namespaces = %v, want %v", got.Namespaces, want)
				}
			}, 5*time.Second, 100*time.Millisecond)
		}
		expectNamespaces("watched-a", "watched-b")

		ns, err := namespaces.Get(ctx, "watched-a", metav1.GetOptions{})
		require.NoError(t, err)
		ns.Labels["scope"] = "ignored"
		_, err = namespaces.Update(ctx, ns, metav1.UpdateOptions{})
		require.NoError(t, err)
		expectNamespaces("watched-b")

		// The discovery listener accepts reads only and does not serve any debug endpoints.
		for _, path := range []string{"/debug/pprof/", "/logging", "/snapshots/krt", "/snapshots/xds"} {
			resp, err := httpClient.Get("http://" + address + path)
			require.NoError(t, err)
			resp.Body.Close()
			require.Equal(t, http.StatusNotFound, resp.StatusCode, path)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, scopeURL, nil)
		require.NoError(t, err)
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	})
}
