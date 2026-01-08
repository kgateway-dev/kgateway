//go:build e2e

package oauth

import (
	"context"
	"fmt"
	"net/http"
	neturl "net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/trafficpolicy"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

const (
	backendURLFirst                = "https://example.com"
	backendURLSecond               = "https://example.com/anything/second"
	logoutURL                      = "https://example.com/logout"
	endSessionEndpoint             = "https://keycloak/realms/master/protocol/openid-connect/logout"
	expectedHttpbinResponseSubstr  = "httpbin"
	expectedAnythingResponseSubstr = "/anything/second"
	tlsPort                        = "443"
	backendHostPort                = "example.com:443"
	keycloakHost                   = "keycloak:443"
	clientUsername                 = "kgateway"
	clientPassword                 = "kgateway"
	nonOAuthBackendURL             = "https://test.com"
	nonOAuthBackendHostPort        = "test.com:443"
)

var (
	_ e2e.NewSuiteFunc = NewTestingSuite

	setup = base.TestCase{
		Manifests: []string{
			filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml"),
			filepath.Join(fsutils.MustGetThisDir(), "testdata", "backend.yaml"),
			filepath.Join(fsutils.MustGetThisDir(), "testdata", "route-2.yaml"),
		},
	}

	gateway  = types.NamespacedName{Name: "oauth-gateway", Namespace: metav1.NamespaceDefault}
	keycloak = types.NamespacedName{Name: "keycloak", Namespace: metav1.NamespaceDefault}
)

type tsuite struct {
	*base.BaseTestingSuite
	gatewayAddr  string
	keycloakAddr string
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &tsuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, nil, base.WithMinGwApiVersion(base.GwApiRequireBackendTLSPolicy)),
	}
}

func (s *tsuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()

	// If the suite should be skipped due to version requirements, don't do additional setup
	// This is usually handled in the base suite setup, but we also need to skip this additional setup.
	if s.SkipSuite() {
		return
	}

	var gwIP, keycloakIP string
	var err error
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		gwIP, err = s.getServiceExternalIP(gateway)
		assert.NoError(c, err)
		s.gatewayAddr = gwIP + ":" + tlsPort

		keycloakIP, err = s.getServiceExternalIP(keycloak)
		assert.NoError(c, err)
		s.keycloakAddr = keycloakIP + ":" + tlsPort
	}, 15*time.Second, 500*time.Millisecond, "failed to get external IPs for gateway or keycloak service")
}

func (s *tsuite) TestOIDC() {
	ctx := s.T().Context()
	r := s.Require()

	client := newClient(map[string]string{
		backendHostPort: s.gatewayAddr,
		keycloakHost:    s.keycloakAddr,
	})

	s.T().Logf("testing OIDC flow for backend %s", backendURLFirst)
	// Login to the Auth server (Keycloak) and exercise the OIDC flow to access the protected backend (httpbin fronted by example.com)
	r.EventuallyWithT(func(c *assert.CollectT) {
		resp, err := client.Login(ctx, backendURLFirst,
			// form data
			map[string]string{"username": clientUsername, "password": clientPassword, "credentialId": ""})

		require.NoError(c, err)
		require.NotNil(c, resp)
		require.Equal(c, http.StatusOK, resp.StatusCode)
		require.Contains(c, string(resp.Body), expectedHttpbinResponseSubstr)
	}, 15*time.Second, 500*time.Millisecond, "login failed")

	// Verify that the session cookies are set
	url, err := neturl.Parse(backendURLFirst)
	r.NoError(err)
	cookies := client.Jar.Cookies(url)
	foundCookies := s.assertCookies(r, cookies)
	s.T().Logf("found session cookies for %s: %v", backendURLFirst, foundCookies)

	// Attempt to access the backend again without needing to login again; disable redirects
	resp, err := client.Get(ctx, backendURLFirst, false)
	r.NoError(err)
	r.NotNil(resp)
	r.Equal(http.StatusOK, resp.StatusCode)
	r.Contains(string(resp.Body), expectedHttpbinResponseSubstr)

	// Initiate a logout with redirects disabled and verify that the set-cookie headers are present to delete
	// the session cookies
	resp, err = client.Get(ctx, logoutURL, false)
	r.NoError(err)
	r.NotNil(resp)
	r.Equal(http.StatusFound, resp.StatusCode) // expect a redirect to the post-logout redirect URI
	location := resp.Headers["Location"]
	r.NotEmpty(location)
	r.Contains(location[0], endSessionEndpoint)
	setCookies := resp.Headers["Set-Cookie"]
	r.NotEmpty(setCookies)
	for _, cookieName := range foundCookies {
		found := false
		for _, setCookie := range setCookies {
			if strings.HasPrefix(setCookie, cookieName+"=deleted;") {
				found = true
				break
			}
		}
		r.True(found, "logout did not delete cookie %s", cookieName)
	}

	// Access backendURLSecond which is protected on a different path and verify its cookies.
	// Since we disabled redirects in the above logout request, the authentication will succeeds without
	// a redirect to the Login page. It will still redirect to the Auth server since session cookies
	// have not been established for the new route yet
	resp, err = client.Get(ctx, backendURLSecond, true)
	r.NoError(err)
	r.NotNil(resp)
	r.Equal(http.StatusOK, resp.StatusCode)
	r.Contains(string(resp.Body), expectedAnythingResponseSubstr)
	url, err = neturl.Parse(backendURLSecond)
	r.NoError(err)
	cookies = client.Jar.Cookies(url)
	foundCookies = s.assertCookies(r, cookies)
	s.T().Logf("found session cookies for %s: %v", backendURLSecond, foundCookies)

	// Verify RP initiated logout by following redirects
	resp, err = client.Get(ctx, logoutURL, true)
	r.NoError(err)
	r.NotNil(resp)
	// TODO: enable this check after determining why sometimes the /logout returns invalid_redirect_uri
	// r.Equal(http.StatusOK, resp.StatusCode)
}

func (s *tsuite) TestNonOAuthBackend() {
	ctx := s.T().Context()
	r := s.Require()

	client := newClient(map[string]string{
		nonOAuthBackendHostPort: s.gatewayAddr,
	})

	// Access a non-OAuth protected backend through the gateway; disable redirects since we don't expect any
	followRedirects := false
	r.EventuallyWithTf(func(c *assert.CollectT) {
		resp, err := client.Get(ctx, nonOAuthBackendURL, followRedirects)
		require.NoError(c, err)
		require.NotNil(c, resp)
		require.Equal(c, http.StatusOK, resp.StatusCode)
		require.Contains(c, string(resp.Body), expectedHttpbinResponseSubstr)
	}, 10*time.Second, 500*time.Millisecond, "accessing non-OAuth backend %s failed", nonOAuthBackendURL)
}

func (s *tsuite) getServiceExternalIP(ref types.NamespacedName) (string, error) {
	svc := &corev1.Service{}
	err := s.TestInstallation.ClusterContext.Client.Get(s.T().Context(), ref, svc)
	if err != nil {
		return "", err
	}
	if len(svc.Status.LoadBalancer.Ingress) == 0 {
		return "", fmt.Errorf("service %s has no external IP", ref)
	}
	return svc.Status.LoadBalancer.Ingress[0].IP, nil
}

func (s *tsuite) assertCookies(r *require.Assertions, cookies []*http.Cookie) []string {
	r.NotEmpty(cookies)

	expectedCookiePrefixes := []string{
		trafficpolicy.OauthAccessTokenCookiePrefix,
		trafficpolicy.OauthIdTokenCookiePrefix,
		trafficpolicy.OauthRefreshTokenCookiePrefix,
		trafficpolicy.OauthHMACCookiePrefix,
	}
	var foundCookies []string
	for _, cookiePrefix := range expectedCookiePrefixes {
		found := false
		for _, cookie := range cookies {
			if strings.HasPrefix(cookie.Name, cookiePrefix) {
				found = true
				foundCookies = append(foundCookies, cookie.Name)
				r.NotEmpty(cookie.Value, "cookie %s should have a value", cookiePrefix)
				break
			}
		}
		r.True(found, "cookie %s not found", cookiePrefix)
	}
	return foundCookies
}
