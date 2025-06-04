package dfp

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/onsi/gomega"
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
		testdefaults.CurlPodManifest,
		simpleServiceManifest,
	}
	s.commonResources = []client.Object{
		// resources from curl manifest
		testdefaults.CurlPod,
		// resources from service manifest
		simpleSvc, simpleDeployment,
		// deployer-generated resources
		proxyDeployment, proxyService,
		dfpRoute, dfpBackend,
	}

	// set up common resources once
	for _, manifest := range s.commonManifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err, "can apply "+manifest)
	}
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, s.commonResources...)

	// make sure pods are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})

	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyObjMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", proxyObjMeta.GetName()),
	}, time.Minute*2)
}

func (s *testingSuite) TearDownSuite() {
	// clean up common resources
	for _, manifest := range s.commonManifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.Require().NoError(err, "can delete "+manifest)
	}
	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, s.commonResources...)

	s.testInstallation.Assertions.EventuallyPodsNotExist(s.ctx, proxyObjMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", proxyObjMeta.GetName()),
	})
}

// TestExtAuthPolicy tests the basic ExtAuth functionality with header-based allow/deny
// Checks for gateay level auth with route level opt out
func (s *testingSuite) TestDynamicForwardProxyBackend() {
	// Wait for pods to be running
	s.ensureBasicRunning()

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
			hostname:                     "simple-svc.kgateway-test.svc.cluster.local",
			expectedStatus:               http.StatusOK,
			expectedUpstreamBodyContents: "X-Header",
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
			s.testInstallation.Assertions.AssertEventualCurlResponse(
				s.ctx,
				testdefaults.CurlPodExecOpt,
				opts,
				&testmatchers.HttpResponse{
					StatusCode: tc.expectedStatus,
					Body:       gomega.ContainSubstring(tc.expectedUpstreamBodyContents),
				})
		})
	}
}

func (s *testingSuite) ensureBasicRunning() {
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=curl",
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyObjMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=super-gateway",
	}, time.Minute)
}
