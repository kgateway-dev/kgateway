package trafficpolicy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	apifake "github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	k8splugin "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/kubernetes"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// TestGatewayExtensionRecoversFromOIDCDiscoveryFailure is the regression test for
// https://github.com/kgateway-dev/kgateway/issues/14497. It drives the real krt collection
// built from TranslateGatewayExtensionBuilder, so it covers the mechanism the fix depends on
// end to end: the transform registering as a dependant of the discovery trigger (including on
// the error path), the background loop re-discovering, and the resulting recomputation clearing
// TrafficPolicyGatewayExtensionIR.Err.
//
// Unlike the discoverer-level tests, this fails if markDependant is dropped from
// buildOAuth2ProviderConfig or moved after the get() error return.
func TestGatewayExtensionRecoversFromOIDCDiscoveryFailure(t *testing.T) {
	ctx := t.Context()

	// Serve the 521 from the issue report until the test marks the provider healthy, mirroring
	// an IdP that is still starting up while the control plane translates.
	var healthy atomic.Bool
	var requestCount int64
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		if !healthy.Load() {
			w.WriteHeader(521)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(oidcProviderConfig{
			TokenEndpoint:         "https://idp.example.com/token",
			AuthorizationEndpoint: "https://idp.example.com/auth",
		})
	}))
	defer idp.Close()

	gwExt := &kgateway.GatewayExtension{
		ObjectMeta: metav1.ObjectMeta{Name: "dex-auth", Namespace: "default"},
		Spec: kgateway.GatewayExtensionSpec{
			Type: new(kgateway.GatewayExtensionTypeOAuth2),
			OAuth2: &kgateway.OAuth2Provider{
				IssuerURI:  new(idp.URL),
				LogoutPath: "/logout",
				BackendRef: gwv1.BackendRef{
					BackendObjectReference: gwv1.BackendObjectReference{
						Kind: new(gwv1.Kind("Service")),
						Name: "dex",
						Port: new(gwv1.PortNumber(80)),
					},
				},
				Credentials: kgateway.OAuth2Credentials{
					ClientID:        "kgateway",
					ClientSecretRef: corev1.LocalObjectReference{Name: "dex-client-secret"},
				},
			},
		},
	}

	fakeClient := apifake.NewClient(t,
		gwExt,
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "dex", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt32(80)}},
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "dex-client-secret", Namespace: "default"},
			Data:       map[string][]byte{clientSecretKey: []byte("shhh")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      wellknown.OAuth2HMACSecret.Name,
				Namespace: wellknown.OAuth2HMACSecret.Namespace,
			},
			Data: map[string][]byte{wellknown.OAuth2HMACSecretKey: []byte("hmac")},
		},
	)

	settings := apisettings.Settings{}
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)
	commoncol, err := collections.NewCommonCollections(
		ctx, krtopts, fakeClient, wellknown.DefaultGatewayControllerName, settings,
	)
	require.NoError(t, err)
	// The kubernetes plugin contributes Service-backed Backends, which the OAuth2 backendRef
	// above resolves against.
	commoncol.InitPlugins(ctx, k8splugin.NewPlugin(ctx, commoncol), settings)

	// Build the discoverer exactly as TranslateGatewayExtensionBuilder does, but with short
	// intervals so the test does not wait out the production 30s retry floor.
	discoverer := newOIDCProviderConfigDiscoverer(
		func() []string { return oidcIssuerURIs(commoncol.GatewayExtensions.List()) },
	)
	discoverer.cacheRefreshInterval = 200 * time.Millisecond
	discoverer.failureRetryInterval = 20 * time.Millisecond
	go discoverer.run(ctx)

	extensions := krt.NewCollection(commoncol.GatewayExtensions, gatewayExtensionBuilder(ctx, commoncol, discoverer))

	fakeClient.RunAndWait(ctx.Done())
	require.Eventually(t, func() bool {
		return commoncol.HasSynced() && commoncol.BackendIndex.HasSynced() && extensions.HasSynced()
	}, 10*time.Second, 50*time.Millisecond, "collections should sync")

	extName := krt.Named{Name: gwExt.Name, Namespace: gwExt.Namespace}.ResourceName()
	getIR := func() *TrafficPolicyGatewayExtensionIR {
		return extensions.GetKey(extName)
	}

	// While the provider is down, the extension carries the discovery failure. This is the
	// state the issue reports as permanent.
	require.Eventually(t, func() bool {
		out := getIR()
		return out != nil && out.Err != nil
	}, 10*time.Second, 50*time.Millisecond, "extension should report the discovery failure")
	require.ErrorContains(t, getIR().Err, "unexpected status code 521")
	require.Nil(t, getIR().OAuth2, "no OAuth2 config should be produced while discovery fails")

	// The provider comes back. Nothing about the Kubernetes objects changes, so only the
	// discovery recompute trigger can drive the extension out of its failed state.
	healthy.Store(true)

	require.Eventually(t, func() bool {
		out := getIR()
		return out != nil && out.Err == nil && out.OAuth2 != nil
	}, 10*time.Second, 20*time.Millisecond,
		"extension should recover once the provider is reachable, without a control plane restart")

	// The recovered config uses the discovered endpoints.
	cfg := getIR().OAuth2.cfg.GetConfig()
	require.Equal(t, "https://idp.example.com/token", cfg.GetTokenEndpoint().GetUri())
	require.Equal(t, "https://idp.example.com/auth", cfg.GetAuthorizationEndpoint())
	require.Greater(t, atomic.LoadInt64(&requestCount), int64(1), "discovery should have been retried")
}

// TestOIDCDiscovererRunStopsOnContextCancel replaces the coverage lost with the old
// refresh() test: run() must exit and stop polling once its context is cancelled.
func TestOIDCDiscovererRunStopsOnContextCancel(t *testing.T) {
	r := require.New(t)

	var requestCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(oidcProviderConfig{
			TokenEndpoint:         "https://example.com/token",
			AuthorizationEndpoint: "https://example.com/auth",
		})
	}))
	defer server.Close()

	issuer := server.URL
	o := newTestDiscoverer(issuer)
	// Expire immediately so every pass re-discovers, making a still-running loop obvious.
	o.cacheRefreshInterval = 0
	o.failureRetryInterval = 5 * time.Millisecond

	_, err := o.get(t.Context(), issuer)
	r.NoError(err)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		o.run(ctx)
	}()

	// Wait until the loop is demonstrably polling.
	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&requestCount) > 2
	}, 5*time.Second, 5*time.Millisecond, "refresh loop should be polling")

	cancel()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not return after context cancellation")
	}

	// A poll the loop had already put on the wire when cancel() landed still reaches the test
	// server's handler, even though client.Do returns "context canceled" without waiting for the
	// response, so requestCount can tick up once more after run() has returned. Wait for it to
	// settle rather than assuming that trailing request has already been counted. This cannot
	// mask a loop that is still running: such a loop polls every failureRetryInterval/2, so it
	// would never produce two equal samples a settle period apart.
	var countAfterStop int64
	require.Eventually(t, func() bool {
		count := atomic.LoadInt64(&requestCount)
		settled := count == countAfterStop
		countAfterStop = count
		return settled
	}, 5*time.Second, 50*time.Millisecond, "polling should stop after cancellation")

	// And it stays stopped.
	time.Sleep(50 * time.Millisecond)
	r.Equal(countAfterStop, atomic.LoadInt64(&requestCount), "no polling should happen after cancellation")
}

// TestProviderBlipDoesNotBreakHealthyExtension is the same scenario driven through the real
// krt collection, using the same harness as
// TestGatewayExtensionRecoversFromOIDCDiscoveryFailure. It asserts a working extension is not
// knocked into an error state -- which rejects every TrafficPolicy referencing it -- by a
// provider blip alone, with no Kubernetes object changing.
func TestProviderBlipDoesNotBreakHealthyExtension(t *testing.T) {
	ctx := t.Context()

	var healthy atomic.Bool
	healthy.Store(true)
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !healthy.Load() {
			w.WriteHeader(521)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(oidcProviderConfig{
			TokenEndpoint:         "https://idp.example.com/token",
			AuthorizationEndpoint: "https://idp.example.com/auth",
		})
	}))
	defer idp.Close()

	gwExt := &kgateway.GatewayExtension{
		ObjectMeta: metav1.ObjectMeta{Name: "dex-auth", Namespace: "default"},
		Spec: kgateway.GatewayExtensionSpec{
			Type: new(kgateway.GatewayExtensionTypeOAuth2),
			OAuth2: &kgateway.OAuth2Provider{
				IssuerURI:  new(idp.URL),
				LogoutPath: "/logout",
				BackendRef: gwv1.BackendRef{
					BackendObjectReference: gwv1.BackendObjectReference{
						Kind: new(gwv1.Kind("Service")),
						Name: "dex",
						Port: new(gwv1.PortNumber(80)),
					},
				},
				Credentials: kgateway.OAuth2Credentials{
					ClientID:        "kgateway",
					ClientSecretRef: corev1.LocalObjectReference{Name: "dex-client-secret"},
				},
			},
		},
	}

	fakeClient := apifake.NewClient(t,
		gwExt,
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "dex", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt32(80)}},
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "dex-client-secret", Namespace: "default"},
			Data:       map[string][]byte{clientSecretKey: []byte("shhh")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      wellknown.OAuth2HMACSecret.Name,
				Namespace: wellknown.OAuth2HMACSecret.Namespace,
			},
			Data: map[string][]byte{wellknown.OAuth2HMACSecretKey: []byte("hmac")},
		},
	)

	settings := apisettings.Settings{}
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)
	commoncol, err := collections.NewCommonCollections(
		ctx, krtopts, fakeClient, wellknown.DefaultGatewayControllerName, settings,
	)
	require.NoError(t, err)
	commoncol.InitPlugins(ctx, k8splugin.NewPlugin(ctx, commoncol), settings)

	discoverer := newOIDCProviderConfigDiscoverer(
		func() []string { return oidcIssuerURIs(commoncol.GatewayExtensions.List()) },
	)
	discoverer.cacheRefreshInterval = 100 * time.Millisecond
	discoverer.failureRetryInterval = 100 * time.Millisecond
	go discoverer.run(ctx)

	extensions := krt.NewCollection(commoncol.GatewayExtensions, gatewayExtensionBuilder(ctx, commoncol, discoverer))

	fakeClient.RunAndWait(ctx.Done())
	require.Eventually(t, func() bool {
		return commoncol.HasSynced() && commoncol.BackendIndex.HasSynced() && extensions.HasSynced()
	}, 10*time.Second, 50*time.Millisecond, "collections should sync")

	extName := krt.Named{Name: gwExt.Name, Namespace: gwExt.Namespace}.ResourceName()
	getIR := func() *TrafficPolicyGatewayExtensionIR { return extensions.GetKey(extName) }

	// Steady state: healthy extension serving a real OAuth2 config.
	require.Eventually(t, func() bool {
		out := getIR()
		return out != nil && out.Err == nil && out.OAuth2 != nil
	}, 10*time.Second, 50*time.Millisecond, "extension should be healthy to begin with")

	// The IdP blips. Nothing about the Kubernetes objects changes.
	healthy.Store(false)

	// Give the refresh loop several passes to do damage, then assert it did not.
	time.Sleep(time.Second)

	out := getIR()
	require.NotNil(t, out)
	if out.Err != nil {
		t.Logf("extension was knocked into an error state by the blip:\n%v", out.Err)
	}
	require.NoError(t, out.Err, "a provider blip must not break a working OAuth2 extension")
	require.NotNil(t, out.OAuth2, "the discovered OAuth2 config must be retained through a blip")
}
