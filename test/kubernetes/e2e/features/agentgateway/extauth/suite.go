package extauth

import (
	"context"
	"net/http"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of tests for ExtAuth functionality
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	// Define the setup TestCase for common resources
	setupTestCase := base.TestCase{
		Manifests: []string{
			testdefaults.CurlPodManifest,
			simpleServiceManifest,
			extAuthManifest,
		},
		Resources: []client.Object{
			// resources from curl manifest
			testdefaults.CurlPod,
			// resources from service manifest
			basicSecureRoute, simpleSvc, simpleDeployment,
			// extauth resources
			extAuthSvc, extAuthExtension,
		},
	}

	// Define test-specific TestCases
	testCases := map[string]base.TestCase{
		"TestExtAuthPolicy": {
			Manifests: []string{
				gatewayWithRouteManifest,
				securedGatewayPolicyManifest,
				insecureRouteManifest,
			},
			Resources: []client.Object{
				// deployer-generated resources
				proxyDeployment, proxyService,
				gatewayAttachedTrafficPolicy,
				insecureRoute,
			},
		},
		"TestRouteTargetedExtAuthPolicy": {
			Manifests: []string{
				gatewayWithRouteManifest,
				securedRouteManifest,
				insecureRouteManifest,
			},
			Resources: []client.Object{
				// deployer-generated resources
				proxyDeployment, proxyService,
				secureRoute, secureTrafficPolicy,
				insecureRoute, insecureTrafficPolicy,
			},
		},
	}

	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuiteWithoutUpgrades(ctx, testInst, setupTestCase, testCases),
	}
}

// TestExtAuthPolicy tests the basic ExtAuth functionality with header-based allow/deny
// Checks for gateway level auth with route level opt out
func (s *testingSuite) TestExtAuthPolicy() {
	// The BaseTestingSuite automatically handles setup and cleanup of test-specific resources

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
		// TODO(npolshak): re-enable once we can disable filters on agentgateway: https://github.com/agentgateway/agentgateway/issues/330
		//{
		//	name:           "request allowed on insecure route",
		//	hostname:       "insecureroute.com",
		//	expectedStatus: http.StatusOK,
		//},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Build curl options
			opts := []curl.Option{
				curl.WithHost(kubeutils.ServiceFQDN(proxyObjMeta)),
				curl.WithHostHeader(tc.hostname),
				curl.WithPort(8080),
			}

			// Add test-specific headers
			for k, v := range tc.headers {
				opts = append(opts, curl.WithHeader(k, v))
			}

			// Test the request
			s.TestInstallation.Assertions.AssertEventualCurlResponse(
				s.Ctx,
				testdefaults.CurlPodExecOpt,
				opts,
				&testmatchers.HttpResponse{
					StatusCode: tc.expectedStatus,
					Body:       gomega.ContainSubstring(tc.expectedUpstreamBodyContents),
				})
		})
	}
}

// TestRouteTargetedExtAuthPolicy tests route level only extauth
func (s *testingSuite) TestRouteTargetedExtAuthPolicy() {
	// The BaseTestingSuite automatically handles setup and cleanup of test-specific resources

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
		// TODO(npolshak): re-enable once we can disable filters on agentgateway: https://github.com/agentgateway/agentgateway/issues/330
		//{
		//	name:           "request allowed on insecure route",
		//	hostname:       "insecureroute.com",
		//	expectedStatus: http.StatusOK,
		//},
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
				curl.WithHost(kubeutils.ServiceFQDN(proxyObjMeta)),
				curl.WithHostHeader(tc.hostname),
				curl.WithPort(8080),
			}

			// Add test-specific headers
			for k, v := range tc.headers {
				opts = append(opts, curl.WithHeader(k, v))
			}

			// Test the request
			s.TestInstallation.Assertions.AssertEventualCurlResponse(
				s.Ctx,
				testdefaults.CurlPodExecOpt,
				opts,
				&testmatchers.HttpResponse{
					StatusCode: tc.expectedStatus,
					Body:       gomega.ContainSubstring(tc.expectedUpstreamBodyContents),
				})
		})
	}
}
