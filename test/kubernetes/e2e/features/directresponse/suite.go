package directresponse

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
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

	// Apply the gateway manifest once for the entire test suite
	err = s.ti.Actions.Kubectl().ApplyFile(s.ctx, gatewayManifest)
	s.NoError(err, "can apply gateway manifest")

	// Check that istio injection is successful and httpbin is running
	s.ti.Assertions.EventuallyObjectsExist(s.ctx, httpbinDeployment)
	// httpbin can take a while to start up with Istio sidecar
	s.ti.Assertions.EventuallyPodsRunning(s.ctx, httpbinDeployment.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=httpbin",
	})
	s.ti.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=curl",
	})

	// Wait for the gateway and proxy to be ready
	s.ti.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	s.ti.Assertions.EventuallyPodsRunning(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gw",
	})

	// Only include functional test manifests - negative test cases moved to gateway translator suite
	s.manifests = map[string][]string{
		"TestBasicDirectResponse": {basicDirectResponseManifests},
		"TestDelegation":          {basicDelegationManifests},
		// "TestInvalidDelegationConflictingFilters": {invalidDelegationConflictingFiltersManifests},
		// "TestInvalidMultipleRouteActions":         {invalidMultipleRouteActionsManifests},
	}
}

func (s *testingSuite) TearDownSuite() {
	err := s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, gatewayManifest)
	s.NoError(err, "can delete gateway manifest")
	err = s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, setupManifest)
	s.NoError(err, "can delete setup manifest")
	err = s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, testdefaults.CurlPodManifest)
	s.NoError(err, "can delete curl pod manifest")
	s.ti.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment, httpbinDeployment)
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

	s.ti.Assertions.EventuallyObjectTypesNotExist(s.ctx, &gwv1.HTTPRouteList{}, &v1alpha1.DirectResponseList{})
}

func (s *testingSuite) TestBasicDirectResponse() {
	// verify that a direct response route works as expected
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
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
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
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
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
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
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
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

// TODO: This test is commented out due to conflicting route actions in the parent HTTPRoute.
// TODO: Re-enable this test once the issue with conflicting filters is resolved or the expected behavior is clarified.
// TODO: When re-enabling, move this test to the gateway translator suite.
// func (s *testingSuite) TestInvalidDelegationConflictingFilters() {
// 	// the parent httproute both 1) specifies a direct response and 2) delegates to another httproute which routes to a service.
// 	// since these route actions are conflicting, we should get a 500 here
// 	s.ti.Assertions.AssertEventualCurlResponse(
// 		s.ctx,
// 		defaults.CurlPodExecOpt,
// 		[]curl.Option{
// 			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
// 			curl.WithHostHeader("www.example.com"),
// 			curl.WithPath("/headers"),
// 		},
// 		&matchers.HttpResponse{
// 			StatusCode: http.StatusInternalServerError,
// 		},
// 		time.Minute,
// 	)

// 	// the parent should show an error in its status
// 	s.ti.Assertions.EventuallyHTTPRouteStatusContainsReason(s.ctx, gwRouteMeta.Name, gwRouteMeta.Namespace,
// 		string(gwv1.RouteReasonIncompatibleFilters), 10*time.Second, 1*time.Second)
// }

// TODO: This test is commented out due to conflicting route actions in the parent HTTPRoute.
// TODO: Re-enable this test once the issue with conflicting filters is resolved or the expected behavior is clarified.
// TODO: When re-enabling, move this test to the gateway translator suite.
// func (s *testingSuite) TestInvalidMultipleRouteActions() {
// 	// the route specifies both a request redirect and a direct response, which is invalid.
// 	// verify the route was replaced with a 500 direct response due to the
// 	// invalid configuration.
// 	s.ti.Assertions.AssertEventualCurlResponse(
// 		s.ctx,
// 		defaults.CurlPodExecOpt,
// 		[]curl.Option{
// 			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
// 			curl.WithHostHeader("www.example.com"),
// 			curl.WithPath("/"),
// 		},
// 		&matchers.HttpResponse{
// 			StatusCode: http.StatusInternalServerError,
// 		},
// 		time.Minute,
// 	)
// 	s.ti.Assertions.EventuallyHTTPRouteStatusContainsReason(s.ctx, httpbinMeta.Name, httpbinMeta.Namespace,
// 		string(gwv1.RouteReasonIncompatibleFilters), 10*time.Second, 1*time.Second)
// }
