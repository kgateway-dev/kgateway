//go:build e2e

package csrf

import (
	"context"
	"net/http"

	"github.com/stretchr/testify/suite"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of basic routing / "happy path" tests
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	setupTestCase := base.TestCase{
		Manifests: []string{commonManifest, testdefaults.CurlPodManifest, testdefaults.HttpbinManifest},
	}
	// everything is applied during setup; there are no additional test-specific manifests
	testCases := map[string]*base.TestCase{}

	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setupTestCase, testCases),
	}
}

func (s *testingSuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()
}

func (s *testingSuite) TestGatewayLevelCSRF() {
	s.setupTest([]string{csrfGwTrafficPolicyManifest}, []client.Object{gwtrafficPolicy})

	// Request without origin header should be allowed (agentgateway CSRF allows this)
	s.assertPreflightResponse("/path1", http.StatusOK, []curl.Option{})

	// Request without origin header should be allowed (agentgateway CSRF allows this)
	s.assertPreflightResponse("/path2", http.StatusOK, []curl.Option{})

	// Request with non-trusted origin header should be rejected
	s.assertPreflightResponse("/path1", http.StatusForbidden, []curl.Option{
		curl.WithHeader("Origin", "example.com"),
	})

	// Request with valid origin header should be allowed (configured in additionalOrigins)
	s.assertPreflightResponse("/path1", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "example.org"),
	})

	// Request with valid origin header should be allowed (configured in additionalOrigins)
	s.assertPreflightResponse("/path2", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "example.org"),
	})
}

func (s *testingSuite) setupTest(manifests []string, resources []client.Object) {
	testutils.Cleanup(s.T(), func() {
		for _, manifest := range manifests {
			err := s.TestInstallation.Actions.Kubectl().DeleteFileSafe(s.Ctx, manifest)
			s.Require().NoError(err)
		}
		s.TestInstallation.Assertions.EventuallyObjectsNotExist(s.Ctx, resources...)
	})

	for _, manifest := range manifests {
		err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, manifest)
		s.Require().NoError(err, "can apply "+manifest)
	}
	s.TestInstallation.Assertions.EventuallyObjectsExist(s.Ctx, resources...)
}

// A safe http method is one that doesn't alter the state of the server (ie read only).
// A CSRF attack targets state changing requests, so the filter only acts on unsafe methods (ones that change state).
// We use POST as the unsafe method to test the filter.
func (s *testingSuite) assertPreflightResponse(path string, expectedStatus int, options []curl.Option) {
	allOptions := append([]curl.Option{
		curl.WithMethod("POST"),
		curl.WithPath(path),
		curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
		curl.WithHostHeader("example.com"),
		curl.WithPort(8080),
	}, options...)

	s.TestInstallation.Assertions.AssertEventuallyConsistentCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		allOptions,
		&testmatchers.HttpResponse{StatusCode: expectedStatus},
	)
}
