//go:build e2e

package dfp

import (
	"context"
	"net/http"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of tests for Dynamic Forward Proxy functionality
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of kgateway
	testInstallation *e2e.TestInstallation

	// manifests shared by all tests
	commonManifests []string
	// resources from manifests shared by all tests
	commonResources []client.Object
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	s.commonManifests = []string{
		gatewayWithRouteManifest,
		simpleServiceManifest,
	}
	s.commonResources = []client.Object{
		// resources from service manifest
		simpleSvc, simpleDeployment,
		// deployer-generated resources
		dfpRoute, dfpBackend,
	}

	// set up common resources once
	for _, manifest := range s.commonManifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err, "can apply "+manifest)
	}
	s.testInstallation.AssertionsT(s.T()).EventuallyObjectsExist(s.ctx, s.commonResources...)
}

func (s *testingSuite) TearDownSuite() {
	if testutils.ShouldSkipCleanup(s.T()) {
		return
	}
	// clean up common resources
	for _, manifest := range s.commonManifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.Require().NoError(err, "can delete "+manifest)
	}
	s.testInstallation.AssertionsT(s.T()).EventuallyObjectsNotExist(s.ctx, s.commonResources...)
}

// TestExtAuthPolicy tests the basic ExtAuth functionality with header-based allow/deny
// Checks for gateay level auth with route level opt out
func (s *testingSuite) TestDynamicForwardProxyBackend() {
	testCases := []struct {
		name                         string
		headers                      map[string]string
		hostname                     string
		expectedStatus               int
		expectedUpstreamBodyContents string
	}{
		{
			name: "request forwarded upstream",
			headers: map[string]string{
				"x-header": "header-value",
			},
			hostname:                     "simple-svc.kgateway-base.svc.cluster.local",
			expectedStatus:               http.StatusOK,
			expectedUpstreamBodyContents: "X-Header",
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
				opts...,
			)
		})
	}
}
