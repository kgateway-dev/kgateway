//go:build e2e

package basicrouting

import (
	"context"
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

var (
	// manifests
	serviceManifest          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "service.yaml")
	headlessServiceManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "headless-service.yaml")
	gatewayWithRouteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway-with-route.yaml")

	// objects
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	// test cases
	// Note: CurlPodManifest removed - using native Go HTTP instead
	setup = base.TestCase{
		Manifests: []string{
			gatewayWithRouteManifest,
		},
	}
	testCases = map[string]*base.TestCase{
		"TestGatewayWithRoute": {
			Manifests: []string{serviceManifest},
		},
		"TestHeadlessService": {
			Manifests: []string{headlessServiceManifest},
		},
	}

	listenerHighPort = 8080
	listenerLowPort  = 80
)

// testingSuite is a suite of basic routing / "happy path" tests
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

func (s *testingSuite) TestGatewayWithRoute() {
	s.assertSuccessfulResponse()
}

func (s *testingSuite) TestHeadlessService() {
	s.assertSuccessfulResponse()
}

func (s *testingSuite) assertSuccessfulResponse() {
	// Use native Go HTTP assertions instead of curl pod.
	// Note: The gateway service must be accessible from the test runner.
	// In cluster environments, this typically means using the service FQDN
	// which the test runner can resolve (requires cluster DNS access or
	// port-forwarding for external test runners).
	for _, port := range []int{listenerHighPort, listenerLowPort} {
	s.BaseTestingSuite.TestInstallation.Assertions.AssertEventualCurlResponseNative(
		s.BaseTestingSuite.Ctx,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPort(port),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring(testdefaults.NginxResponse),
		},
	)
}
