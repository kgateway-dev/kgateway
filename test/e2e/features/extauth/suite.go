//go:build e2e

package extauth

import (
	"context"
	"net/http"
	"strings"
	"time"

	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/envoyutils/admincli"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

const (
	// Signed with the same key as the JWKS in metadata-context-namespaces.yaml.
	// claim has email=dev1@kgateway.io
	metadataJWTDev1Token = "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJodHRwczovL2Rldi5leGFtcGxlLmNvbSIsImV4cCI6NDgwNDMyNDczNiwiaWF0IjoxNjQ4NjUxMTM2LCJvcmciOiJpbnRlcm5hbCIsImVtYWlsIjoiZGV2MUBrZ2F0ZXdheS5pbyIsImdyb3VwIjoiZW5naW5lZXJpbmciLCJzY29wZSI6ImlzOmRldmVsb3BlciJ9.pqzk87Gny6mT8Gk7CVfkminm3u9CrNPhRt0oElwmfwZ7Jak1Ss4iOZ7MSZEgZFPxGiaz3DQyvos65dqbM_e4RaLYXb9fFYylaBl8kE8bhqMnXfPBNp9C4XTsSz4mR-eUvnkXXZ31dhMkoZvwIswWXR50wZ0rC6NF60Tye0sHJRdDcwL5778wDzLnualvtIiL-CbhWzXgRmjcrK3sbikLCHBjQiTEyBMPOVqS5NqJBgd7ZW1UASoxuxjCLsN8tBIaAFSACf8FZggAh9vEUJ_uc39kvOKQ0vs0pxvoYtsMPcndBYhws6IUhx_iF__qs_zz9mDNp8aMbXSlEdJG30wiRA" //gosec:disable G101
	// claim has email=dev2@kgateway.io
	metadataJWTDev2Token = "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJodHRwczovL2Rldi5leGFtcGxlLmNvbSIsImV4cCI6NDgwNDMyNDczNiwiaWF0IjoxNjQ4NjUxMTM2LCJvcmciOiJpbnRlcm5hbCIsImVtYWlsIjoiZGV2MkBrZ2F0ZXdheS5pbyIsImdyb3VwIjoiZW5naW5lZXJpbmciLCJzY29wZSI6ImlzOmRldmVsb3BlciJ9.S0a_Lu2y0gaXBCnO3ydGJCnXt5R-QMxBvJOjYOTzorcnUOcaOTMOd3fUBY8ojZR-f0xTEy6M6K1V0yKxeq6Mys9Le9SE6oabP6gttktnwL5c9e9rzMcmGz1NVyUBav2N8Yiuw7Va8gyIod02vJrllQteMfZSqoAUmDLmpFs3bvkIgMlWDtVAWPqoGJ4ZI-yf0WfTSmW-kFbaiIz4pQNm03Q9M_ZMiHyOTtCDZuc0pSQ0_uvjnqHrefBgJJkFEv58pVqZVJphEOAfl7CpWlT9dXiPVoMhy4RTezkfrjuCqvW7dDwGZGSUqLYDZsOJ8yeIdeW9LKMaGcPag1AbRCe4HQ" //gosec:disable G101
)

// testingSuite is a suite of tests for ExtAuth functionality
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	setup := base.TestCase{
		Manifests: []string{
			routeManifest,
			gatewayWithRouteManifest,
			extAuthManifest,
		},
	}
	testCases := map[string]*base.TestCase{
		"TestExtAuthPolicy": {
			Manifests: []string{securedGatewayPolicyManifest, insecureRouteManifest},
		},
		"TestRouteTargetedExtAuthPolicy": {
			Manifests: []string{securedRouteManifest, insecureRouteManifest},
		},
		// The buffer filter's placement is a property of the whole filter chain, so the staged and
		// default-staged routes cannot share a listener: they run as separate test cases.
		"TestBufferFilterStageEnforcesAheadOfExtAuth": {
			Manifests: []string{bufferedRouteManifest},
		},
		"TestDefaultBufferStageDoesNotEnforceBehindExtAuth": {
			Manifests: []string{bufferedRouteDefaultStageManifest},
		},
		"TestExtAuthMetadataContextNamespaces": {
			Manifests: []string{metadataContextNamespacesManifest},
		},
	}
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

// TestExtAuthPolicy tests the basic ExtAuth functionality with header-based allow/deny
// Checks for gateway level auth with route level opt out
func (s *testingSuite) TestExtAuthPolicy() {
	testCases := []struct {
		name                         string
		headers                      map[string]string
		hostname                     string
		expectedStatus               int
		expectedUpstreamBodyContents string
	}{
		{
			name: "request allowed with allow header",
			headers: map[string]string{
				"x-ext-authz": "allow",
			},
			hostname:                     "example.com",
			expectedStatus:               http.StatusOK,
			expectedUpstreamBodyContents: "X-Ext-Authz-Check-Result",
		},
		{
			name:           "request denied without allow header",
			headers:        map[string]string{},
			hostname:       "example.com",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:     "request denied with deny header",
			hostname: "example.com",
			headers: map[string]string{
				"x-ext-authz": "deny",
			},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "request allowed on insecure route",
			hostname:       "insecureroute.com",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Build curl options
			opts := []curl.Option{
				curl.WithHostHeader(tc.hostname),
				curl.WithPort(80),
			}

			// Add test-specific headers
			for k, v := range tc.headers {
				opts = append(opts, curl.WithHeader(k, v))
			}

			// Test the request
			common.BaseGateway.Send(
				s.T(),
				&testmatchers.HttpResponse{
					StatusCode: tc.expectedStatus,
					Body:       gomega.ContainSubstring(tc.expectedUpstreamBodyContents),
				},
				opts...)
		})
	}
}

// TestRouteTargetedExtAuthPolicy tests route level only extauth
func (s *testingSuite) TestRouteTargetedExtAuthPolicy() {
	testCases := []struct {
		name                         string
		headers                      map[string]string
		hostname                     string
		expectedStatus               int
		expectedUpstreamBodyContents string
	}{
		{
			name:           "request allowed by default",
			headers:        map[string]string{},
			hostname:       "example.com",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "request allowed on insecure route",
			hostname:       "insecureroute.com",
			expectedStatus: http.StatusOK,
		},
		{
			name: "request allowed with allow header on secured route",
			headers: map[string]string{
				"x-ext-authz": "allow",
			},
			hostname:                     "secureroute.com",
			expectedStatus:               http.StatusOK,
			expectedUpstreamBodyContents: "X-Ext-Authz-Check-Result",
		},
		{
			name:           "request denied without header on secured route",
			hostname:       "secureroute.com",
			headers:        map[string]string{},
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Build curl options
			opts := []curl.Option{
				curl.WithHostHeader(tc.hostname),
				curl.WithPort(80),
			}

			// Add test-specific headers
			for k, v := range tc.headers {
				opts = append(opts, curl.WithHeader(k, v))
			}

			// Test the request
			common.BaseGateway.Send(
				s.T(),
				&testmatchers.HttpResponse{
					StatusCode: tc.expectedStatus,
					Body:       gomega.ContainSubstring(tc.expectedUpstreamBodyContents),
				},
				opts...)
		})
	}
}

// bufferedRequest is a request against one of the buffered routes: `maxRequestSize` is 1024 on
// both, and ext_authz reads the body with an 8192-byte limit of its own, so the only filter that
// can reject a body between those sizes is the buffer filter.
type bufferedRequest struct {
	name           string
	hostname       string
	headers        map[string]string
	bodySize       int
	expectedStatus int
}

func (s *testingSuite) sendBufferedRequests(testCases []bufferedRequest) {
	for _, tc := range testCases {
		s.Run(tc.name, func() {
			opts := []curl.Option{
				curl.WithHostHeader(tc.hostname),
				curl.WithPort(80),
				curl.WithBody(strings.Repeat("x", tc.bodySize)),
			}
			for k, v := range tc.headers {
				opts = append(opts, curl.WithHeader(k, v))
			}

			common.BaseGateway.Send(
				s.T(),
				&testmatchers.HttpResponse{
					StatusCode: tc.expectedStatus,
				},
				opts...)
		})
	}
}

// TestBufferFilterStageEnforcesAheadOfExtAuth checks that `buffer.filterStage` makes
// `maxRequestSize` enforce ahead of an ext_authz check that reads the request body: an oversized
// body gets a 413 whether or not the auth service would have allowed it, which is what proves the
// buffer filter runs first.
func (s *testingSuite) TestBufferFilterStageEnforcesAheadOfExtAuth() {
	s.sendBufferedRequests([]bufferedRequest{
		{
			name:           "small body with allow header reaches the backend",
			hostname:       "bufferedroute.com",
			headers:        map[string]string{"x-ext-authz": "allow"},
			bodySize:       512,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "small body without allow header is denied by auth",
			hostname:       "bufferedroute.com",
			bodySize:       512,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "oversized body with allow header is rejected by the buffer filter",
			hostname:       "bufferedroute.com",
			headers:        map[string]string{"x-ext-authz": "allow"},
			bodySize:       1500,
			expectedStatus: http.StatusRequestEntityTooLarge,
		},
		{
			// A 403 here would mean ext_authz ran first.
			name:           "oversized body is rejected before auth gets to deny it",
			hostname:       "bufferedroute.com",
			bodySize:       1500,
			expectedStatus: http.StatusRequestEntityTooLarge,
		},
	})
}

// TestDefaultBufferStageDoesNotEnforceBehindExtAuth pins the behavior `buffer.filterStage` exists
// to work around: with ext_authz reading the body ahead of the buffer filter's default placement,
// `maxRequestSize` is inert and an oversized body reaches the backend. See
// buffered-route-default-stage.yaml.
func (s *testingSuite) TestDefaultBufferStageDoesNotEnforceBehindExtAuth() {
	s.sendBufferedRequests([]bufferedRequest{
		{
			name:           "oversized body with allow header reaches the backend",
			hostname:       "bufferedroute-default.com",
			headers:        map[string]string{"x-ext-authz": "allow"},
			bodySize:       1500,
			expectedStatus: http.StatusOK,
		},
		{
			// Auth runs first at the default placement, so the oversized body is denied, not 413'd.
			name:           "oversized body without allow header is denied by auth",
			hostname:       "bufferedroute-default.com",
			bodySize:       1500,
			expectedStatus: http.StatusForbidden,
		},
	})
}

// TestExtAuthMetadataContextNamespaces proves that an explicit
// metadataContextNamespaces opt-in runs gRPC ext-auth after JWT and forwards the
// namespaces requested by the user. Three routes are used:
//   - metadata-extauth.com sets an explicit empty list, proving the JWT
//     namespace is still included;
//   - metadata-explicit.com sets a custom namespace, proving user-supplied
//     namespaces are preserved alongside the required JWT namespace;
//   - metadata-legacy.com omits the field, proving the historical pre-AuthN
//     placement still forwards Authorization to gRPC ext-auth.
//
// The metadata-based auth server ignores request headers entirely: it only
// allows requests whose `envoy.filters.http.jwt_authn` payload contains
// email=dev1@kgateway.io. The JWT filter is not configured with claimsToHeaders,
// so there is no header the server could read instead.
func (s *testingSuite) TestExtAuthMetadataContextNamespaces() {
	// Assert the decoded ext_authz config, not a raw config-dump substring: this
	// pins both the filter instance and the exact namespace list Envoy received.
	s.TestInstallation.AssertionsT(s.T()).AssertEnvoyAdminApi(
		s.Ctx,
		metav1.ObjectMeta{Name: "gateway", Namespace: "kgateway-base"},
		func(ctx context.Context, adminClient *admincli.Client) {
			const filterName = "ext_auth/kgateway-base/metadata-extauth-explicit/post-authn"
			s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
				listener, err := adminClient.GetSingleListenerFromDynamicListeners(ctx, "listener~80")
				g.Expect(err).NotTo(gomega.HaveOccurred(), "can get listener from config dump")

				var hcm *envoy_hcm.HttpConnectionManager
				for _, chain := range listener.GetFilterChains() {
					for _, filter := range chain.GetFilters() {
						candidate := &envoy_hcm.HttpConnectionManager{}
						if err := filter.GetTypedConfig().UnmarshalTo(candidate); err == nil {
							hcm = candidate
							break
						}
					}
					if hcm != nil {
						break
					}
				}
				g.Expect(hcm).NotTo(gomega.BeNil(), "listener should have an HCM filter")

				var extAuth *envoy_ext_authz_v3.ExtAuthz
				for _, filter := range hcm.GetHttpFilters() {
					if filter.GetName() != filterName {
						continue
					}
					extAuth = &envoy_ext_authz_v3.ExtAuthz{}
					g.Expect(filter.GetTypedConfig().UnmarshalTo(extAuth)).To(gomega.Succeed(),
						"post-AuthN ext_authz filter should decode")
					break
				}
				g.Expect(extAuth).NotTo(gomega.BeNil(), "post-AuthN ext_authz filter should be registered")
				g.Expect(extAuth.GetMetadataContextNamespaces()).To(gomega.Equal([]string{
					"custom.auth.example",
					"envoy.filters.http.jwt_authn",
				}))
			}).WithTimeout(30 * time.Second).WithPolling(time.Second).Should(gomega.Succeed())
		},
	)

	const (
		allowedBody      = "X-Ext-Authz-Check-Result"
		deniedClaimBody  = `email=dev1@kgateway.io not found in "envoy.filters.http.jwt_authn" JWT metadata`
		deniedHeaderBody = `required header "authorization" is missing`
		emptyHost        = "metadata-extauth.com"
		explicitHost     = "metadata-explicit.com"
		legacyHost       = "metadata-legacy.com"
	)

	testCases := []struct {
		name           string
		host           string
		token          string
		headers        map[string]string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "empty list: valid JWT claim in metadata is allowed",
			host:           emptyHost,
			token:          metadataJWTDev1Token,
			expectedStatus: http.StatusOK,
			// Added by the auth server on allow; the backend echoes request headers.
			expectedBody: allowedBody,
		},
		{
			// Both tokens are valid JWTs; only their payloads differ. A 403 here
			// proves the auth server saw the payload from metadata_context.
			name:           "empty list: different verified JWT claim in metadata is denied",
			host:           emptyHost,
			token:          metadataJWTDev2Token,
			expectedStatus: http.StatusForbidden,
			expectedBody:   deniedClaimBody,
		},
		{
			// The sample ext-auth server used by the other tests in this suite
			// would allow this request. Here it must still be denied: the auth
			// decision only reads JWT metadata, not client-supplied headers.
			name:           "empty list: allow header cannot override metadata claim mismatch",
			host:           emptyHost,
			token:          metadataJWTDev2Token,
			headers:        map[string]string{"x-ext-authz": "allow"},
			expectedStatus: http.StatusForbidden,
			expectedBody:   deniedClaimBody,
		},
		{
			name:           "empty list: missing JWT is rejected before ext auth",
			host:           emptyHost,
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "Jwt is missing",
		},
		{
			// A configured namespace must not displace the required JWT namespace.
			name:           "explicit list: valid JWT claim in metadata is allowed",
			host:           explicitHost,
			token:          metadataJWTDev1Token,
			expectedStatus: http.StatusOK,
			expectedBody:   allowedBody,
		},
		{
			name:           "explicit list: different verified JWT claim in metadata is denied",
			host:           explicitHost,
			token:          metadataJWTDev2Token,
			expectedStatus: http.StatusForbidden,
			expectedBody:   deniedClaimBody,
		},
		{
			name:           "explicit list: missing JWT is rejected before ext auth",
			host:           explicitHost,
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "Jwt is missing",
		},
		{
			// With the field omitted, gRPC ext-auth keeps its historical position
			// before JWT and can still see Authorization.
			name:           "omitted field: bearer header reaches gRPC ext-auth",
			host:           legacyHost,
			token:          metadataJWTDev1Token,
			expectedStatus: http.StatusOK,
			expectedBody:   allowedBody,
		},
		{
			name:           "omitted field: request without bearer header is denied by ext auth",
			host:           legacyHost,
			expectedStatus: http.StatusForbidden,
			expectedBody:   deniedHeaderBody,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			opts := []curl.Option{
				curl.WithHostHeader(tc.host),
			}
			if tc.token != "" {
				opts = append(opts, curl.WithHeader("Authorization", "Bearer "+tc.token))
			}
			for k, v := range tc.headers {
				opts = append(opts, curl.WithHeader(k, v))
			}

			common.BaseGateway.Send(
				s.T(),
				&testmatchers.HttpResponse{
					StatusCode: tc.expectedStatus,
					Body:       gomega.ContainSubstring(tc.expectedBody),
				},
				opts...)
		})
	}
}
