//go:build e2e

package basicrouting

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

var (
	// manifests
	serviceManifest          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "service.yaml")
	headlessServiceManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "headless-service.yaml")
	gatewayWithRouteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway-with-route.yaml")
	routeMissingGwManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route-missing-gw.yaml")

	// objects
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	// test cases
	setup = base.TestCase{
		Manifests: []string{
			testdefaults.CurlPodManifest,
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
		"TestClearStaleStatus": {
			Manifests: []string{serviceManifest},
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

func (s *testingSuite) TestClearStaleStatus() {
	// Verify route has parent status true
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		"example-route",
		"default",
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)

	// Modify route to reference missing-gw
	err := s.TestInstallation.Actions.Kubectl().ApplyFile(
		s.Ctx,
		routeMissingGwManifest,
	)
	s.Require().NoError(err)

	// Verify the parent status for "gw" is removed
	s.assertParentStatusRemoved("gw")

	// Re-apply the correct parent ref
	err = s.TestInstallation.Actions.Kubectl().ApplyFile(
		s.Ctx,
		gatewayWithRouteManifest,
	)
	s.Require().NoError(err)
}

func (s *testingSuite) assertSuccessfulResponse() {
	for _, port := range []int{listenerHighPort, listenerLowPort} {
		s.TestInstallation.Assertions.AssertEventualCurlResponse(
			s.Ctx,
			testdefaults.CurlPodExecOpt,
			[]curl.Option{
				curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
				curl.WithHostHeader("example.com"),
				curl.WithPort(port),
			},
			&testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Body:       gomega.ContainSubstring(testdefaults.NginxResponse),
			})
	}
}

func (s *testingSuite) assertParentStatusRemoved(parentName string) {
	currentTimeout, pollingInterval := helpers.GetTimeouts()
	s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
		route := &gwv1.HTTPRoute{}
		err := s.TestInstallation.ClusterContext.Client.Get(
			s.Ctx,
			types.NamespacedName{Name: "example-route", Namespace: "default"},
			route,
		)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get HTTPRoute")

		// Check that no parent status exists for the original "gw" gateway
		for _, parent := range route.Status.Parents {
			g.Expect(string(parent.ParentRef.Name)).NotTo(gomega.Equal(parentName),
				fmt.Sprintf("parent status for gateway %s should be removed. Full status: %+v", parentName, route.Status))
		}
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}
