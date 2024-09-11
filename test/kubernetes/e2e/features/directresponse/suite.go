package directresponse

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/defaults"
	testdefaults "github.com/solo-io/gloo/test/kubernetes/e2e/defaults"
)

type testingSuite struct {
	suite.Suite
	ctx context.Context
	ti  *e2e.TestInstallation
	// maps test name to a list of manifests to apply before the test
	manifests map[string][]string
}

func NewTestingSuite(
	ctx context.Context,
	testInst *e2e.TestInstallation,
) suite.TestingSuite {
	return &testingSuite{
		ctx: ctx,
		ti:  testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	// Check that the common setup manifest is applied
	err := s.ti.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.NoError(err, "can apply "+setupManifest)
	err = s.ti.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.CurlPodManifest)
	s.NoError(err, "can apply curl pod manifest")

	// Check that istio injection is successful and httpbin is running
	s.ti.Assertions.EventuallyObjectsExist(s.ctx, httpbinDeployment)
	// httpbin can take a while to start up with Istio sidecar
	s.ti.Assertions.EventuallyPodsRunning(s.ctx, httpbinDeployment.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=httpbin",
	})
	s.ti.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=curl",
	})

	// include gateway manifests for the tests, so we recreate it for each test run
	s.manifests = map[string][]string{
		"TestBasicDirectResponse":                 {gatewayManifest, basicDirectResposeManifests},
		"TestDelegation":                          {gatewayManifest, basicDelegationManifests},
		"TestInvalidDelegationConflictingFilters": {gatewayManifest, invalidDelegationConflictingFiltersManifests},
		"TestInvalidMissingRef":                   {gatewayManifest, invalidMissingRefManifests},
		"TestInvalidOverlappingFilters":           {gatewayManifest, invalidOverlappingFiltersManifests},
		"TestInvalidMultipleRouteActions":         {gatewayManifest, invalidMultipleRouteActionsManifests},
		"TestInvalidBackendRefFilter":             {gatewayManifest, invalidBackendRefFilterManifests},
	}
}

func (s *testingSuite) TearDownSuite() {
	err := s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, setupManifest)
	s.NoError(err, "can delete setup manifest")
	err = s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, testdefaults.CurlPodManifest)
	s.NoError(err, "can delete curl pod manifest")
	s.ti.Assertions.EventuallyObjectsNotExist(s.ctx, httpbinDeployment)
}

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for %s, manifest map contents: %v", testName, s.manifests)
	}
	for _, manifest := range manifests {
		err := s.ti.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Assert().NoError(err, "can apply manifest "+manifest)
	}

	// we recreate the `Gateway` resource (and thus dynamically provision the proxy pod) for each test run
	// so let's assert the proxy svc and pod is ready before moving on
	s.ti.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	s.ti.Assertions.EventuallyPodsRunning(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gloo-proxy-gw",
	})
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for " + testName)
	}

	for _, manifest := range manifests {
		output, err := s.ti.Actions.Kubectl().DeleteFileWithOutput(s.ctx, manifest)
		s.ti.Assertions.ExpectObjectDeleted(manifest, err, output)
	}
}

func (s *testingSuite) TestBasicDirectResponse() {
	// verify that a direct response route works as expected
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(glooProxyObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/robots.txt"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       ContainSubstring("Disallow: /custom"),
		},
		time.Minute,
	)
}

func (s *testingSuite) TestDelegation() {
	// verify the regular child route works as expected.
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(glooProxyObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/headers"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       ContainSubstring(`"headers"`),
		},
		time.Minute,
	)

	// verify the parent's DR works as expected.
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(glooProxyObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/parent"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusFound,
			Body:       ContainSubstring(`Hello from parent`),
		},
		time.Minute,
	)

	// verify that the child's DR works as expected.
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(glooProxyObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/child"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusFound,
			Body:       ContainSubstring(`Hello from child`),
		},
		time.Minute,
	)
}

func (s *testingSuite) TestInvalidDelegationConflictingFilters() {
	// verify that the child's DR works as expected.
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(glooProxyObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/headers"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusInternalServerError,
		},
		time.Minute,
	)
}

func (s *testingSuite) TestInvalidMissingRef() {
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(glooProxyObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/non-existent"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusInternalServerError,
		},
		time.Minute,
	)
}

func (s *testingSuite) TestInvalidOverlappingFilters() {
	// verify that the route was replaced with a 500 direct response due to the
	// invalid configuration.
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(glooProxyObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusInternalServerError,
		},
		time.Minute,
	)
	c := s.ti.ClusterContext.Client
	s.Require().EventuallyWithT(func(t *assert.CollectT) {
		route := &gwv1.HTTPRoute{}
		err := c.Get(s.ctx, client.ObjectKeyFromObject(httpbinDeployment), route)
		assert.NoError(t, err, "route not found")
		s.ti.Assertions.AssertHTTPRouteStatusContainsReason(route, string(gwv1.RouteReasonBackendNotFound))
	}, 10*time.Second, 1*time.Second)
}

func (s *testingSuite) TestInvalidMultipleRouteActions() {
	// verify the route was replaced with a 500 direct response due to the
	// invalid configuration.
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(glooProxyObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusInternalServerError,
		},
		time.Minute,
	)
	c := s.ti.ClusterContext.Client
	s.Require().EventuallyWithT(func(t *assert.CollectT) {
		route := &gwv1.HTTPRoute{}
		err := c.Get(s.ctx, client.ObjectKeyFromObject(httpbinDeployment), route)
		assert.NoError(t, err, "route not found")
		s.ti.Assertions.AssertHTTPRouteStatusContainsReason(route, string(gwv1.RouteReasonIncompatibleFilters))
	}, 10*time.Second, 1*time.Second)
}

func (s *testingSuite) TestInvalidBackendRefFilter() {
	// verify that configuring a DR with a backendRef filter results in a 404 as
	// this configuration is not supported / ignored by the direct response plugin.
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(glooProxyObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/not-implemented"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusNotFound,
			Body:       ContainSubstring(`Not Found (go-httpbin does not handle the path /not-implemented)`),
		},
		time.Minute,
	)
}
