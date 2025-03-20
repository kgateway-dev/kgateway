package extauth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of tests for ExtAuth functionality
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of kgateway
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

// TestExtAuthPolicy tests the basic ExtAuth functionality with header-based allow/deny
func (s *testingSuite) TestExtAuthPolicy() {
	// Setup test resources
	manifests := []string{
		testdefaults.CurlPodManifest,
		simpleServiceManifest,
		gatewayWithRouteManifest,
		extAuthServiceManifest,
	}
	manifestObjects := []client.Object{
		testdefaults.CurlPod,                               // curl pod for testing
		simpleSvc,                                          // echo service
		proxyService, proxyServiceAccount, proxyDeployment, // proxy
		extAuthSvc,       // extauth service
		extAuthExtension, // extauth extension
	}

	// Cleanup after test
	s.T().Cleanup(func() {
		for _, manifest := range manifests {
			err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
			s.Require().NoError(err)
		}
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, manifestObjects...)
	})

	// Apply all manifests
	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
	}
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, manifestObjects...)

	// Wait for pods to be running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=curl",
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gw",
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, extAuthSvc.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=extauth",
	})

	testCases := []struct {
		name            string
		headers         map[string]string
		expectedStatus  int
		expectedHeaders map[string]interface{}
	}{
		{
			name: "request allowed with allow header",
			headers: map[string]string{
				"x-ext-authz": "allow",
			},
			expectedStatus: http.StatusOK,
			expectedHeaders: map[string]interface{}{
				"x-ext-authz-result": "allowed",
			},
		},
		{
			name:           "request denied without allow header",
			headers:        map[string]string{},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "request denied with deny header",
			headers: map[string]string{
				"x-ext-authz": "deny",
			},
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Build curl options
			opts := []curl.Option{
				curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
				curl.WithHostHeader("example.com"),
				curl.WithPort(8080),
			}

			// Add test-specific headers
			for k, v := range tc.headers {
				opts = append(opts, curl.WithHeader(k, v))
			}

			// Test the request
			s.testInstallation.Assertions.AssertEventualCurlResponse(
				s.ctx,
				testdefaults.CurlPodExecOpt,
				opts,
				&testmatchers.HttpResponse{
					StatusCode: tc.expectedStatus,
					Headers:    tc.expectedHeaders,
				})
		})
	}
}

// TestExtAuthWithRequestBody tests the ExtAuth route policy with request body buffering
func (s *testingSuite) TestExtAuthWithRequestBody() {
	manifests := []string{
		testdefaults.CurlPodManifest,
		simpleServiceManifest,
		gatewayWithRouteManifest,
		extAuthServiceManifest,
		extAuthExtensionManifest,
		routePolicyWithExtAuthRequestBody,
	}
	manifestObjects := []client.Object{
		testdefaults.CurlPod,                               // curl
		simpleSvc,                                          // echo service
		proxyService, proxyServiceAccount, proxyDeployment, // proxy
		extAuthSvc,       // extauth service
		extAuthExtension, // extauth extension
	}

	s.T().Cleanup(func() {
		for _, manifest := range manifests {
			err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
			s.Require().NoError(err)
		}
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, manifestObjects...)
	})

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
	}
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, manifestObjects...)

	// make sure pods are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=curl",
	})

	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gw",
	})

	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, extAuthSvc.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=extauth",
	})

	testCases := []struct {
		name string
		opts []curl.Option
		resp *testmatchers.HttpResponse
	}{
		{
			name: "request body included in auth check",
			opts: []curl.Option{
				curl.WithBody(`{"action": "allow", "token": "valid-token"}`),
				curl.WithHeader("Content-Type", "application/json"),
				curl.WithHeader("Authorization", "Bearer valid-token"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]interface{}{
					"x-auth-status": "authorized",
				},
			},
		},
		{
			name: "request body too large",
			opts: []curl.Option{
				curl.WithBody(fmt.Sprintf(`{"action": "allow", "token": "valid-token", "data": "%s"}`, make([]byte, 1025))),
				curl.WithHeader("Content-Type", "application/json"),
				curl.WithHeader("Authorization", "Bearer valid-token"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusRequestEntityTooLarge,
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.testInstallation.Assertions.AssertEventualCurlResponse(
				s.ctx,
				testdefaults.CurlPodExecOpt,
				append(tc.opts,
					curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
					curl.WithHostHeader("example.com"),
					curl.WithPort(8080),
				),
				tc.resp)
		})
	}
}
