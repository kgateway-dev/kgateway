package route_delegation

import (
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type tsuite struct {
	suite.Suite

	ctx context.Context
	// ti contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	ti *e2e.TestInstallation

	// maps test name to a list of manifests to apply before the test
	manifests map[string][]string

	manifestObjects map[string][]client.Object

	// resources from common manifest
	commonResources []client.Object
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &tsuite{
		ctx: ctx,
		ti:  testInst,
		manifests: map[string][]string{
			"TestBasic":                       {basicRoutesManifest},
			"TestRecursive":                   {recursiveRoutesManifest},
			"TestCyclic":                      {cyclicRoutesManifest},
			"TestInvalidChild":                {invalidChildRoutesManifest},
			"TestHeaderQueryMatch":            {headerQueryMatchRoutesManifest},
			"TestMultipleParents":             {multipleParentsManifest},
			"TestInvalidChildValidStandalone": {invalidChildValidStandaloneManifest},
			"TestUnresolvedChild":             {unresolvedChildManifest},
			"TestMatcherInheritance":          {matcherInheritanceManifest},
			"TestRouteWeight":                 {routeWeightManifest},
			"TestPolicyMerging":               {policyMergingManifest},
		},
	}
}

func (s *tsuite) SetupSuite() {
	// Not every resource that is applied needs to be verified. We are not testing `kubectl apply`,
	// but the below code demonstrates how it can be done if necessary
	s.manifestObjects = map[string][]client.Object{
		basicRoutesManifest:                 {routeRoot, routeTeam1, routeTeam2},
		cyclicRoutesManifest:                {routeRoot, routeTeam1, routeTeam2},
		recursiveRoutesManifest:             {routeRoot, routeTeam1, routeTeam2},
		invalidChildRoutesManifest:          {routeRoot, routeTeam1, routeTeam2},
		headerQueryMatchRoutesManifest:      {routeRoot, routeTeam1, routeTeam2},
		multipleParentsManifest:             {routeParent1, routeParent2, routeTeam1, routeTeam2},
		invalidChildValidStandaloneManifest: {proxyTestService, proxyTestDeployment, routeRoot, routeTeam1, routeTeam2},
		unresolvedChildManifest:             {routeRoot},
		matcherInheritanceManifest:          {routeParent1, routeParent2, routeTeam1},
	}

	s.commonResources = []client.Object{
		// resources from manifest
		httpbinTeam1Service, httpbinTeam1Deployment, httpbinTeam2Service, httpbinTeam2Deployment, gateway,
		// deployer-generated resources
		proxyDeployment, proxyService,
	}

	// set up common resources once
	err := s.ti.Actions.Kubectl().ApplyFile(s.ctx, commonManifest)
	s.Require().NoError(err, "can apply common manifest")
	s.ti.Assertions.EventuallyObjectsExist(s.ctx, s.commonResources...)
	// make sure pods are running
	s.ti.Assertions.EventuallyPodsRunning(s.ctx, httpbinTeam1Deployment.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=httpbin,version=v1",
	})
	s.ti.Assertions.EventuallyPodsRunning(s.ctx, httpbinTeam2Deployment.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=httpbin,version=v2",
	})
	s.ti.Assertions.EventuallyPodsRunning(s.ctx, proxyMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", proxyMeta.GetName()),
	})

	// set up curl once
	err = s.ti.Actions.Kubectl().ApplyFile(s.ctx, defaults.CurlPodManifest)
	s.Require().NoError(err, "can apply curl pod manifest")
	s.ti.Assertions.EventuallyPodsRunning(s.ctx, defaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.CurlPodLabelSelector,
	})
}

func (s *tsuite) TearDownSuite() {
	// clean up curl
	err := s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, defaults.CurlPodManifest)
	s.Require().NoError(err, "can delete curl pod manifest")
	s.ti.Assertions.EventuallyObjectsNotExist(s.ctx, defaults.CurlPod)

	// clean up common resources
	err = s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, commonManifest)
	s.Require().NoError(err, "can delete common manifest")
	s.ti.Assertions.EventuallyObjectsNotExist(s.ctx, s.commonResources...)
	s.ti.Assertions.EventuallyPodsNotExist(s.ctx, httpbinTeam1Deployment.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=httpbin,version=v1",
	})
	s.ti.Assertions.EventuallyPodsNotExist(s.ctx, httpbinTeam2Deployment.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=httpbin,version=v2",
	})
	s.ti.Assertions.EventuallyPodsNotExist(s.ctx, proxyMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", proxyMeta.GetName()),
	})
}

func (s *tsuite) BeforeTest(suiteName, testName string) {
	manifests := s.manifests[testName]
	for _, manifest := range manifests {
		err := s.ti.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
		s.ti.Assertions.EventuallyObjectsExist(s.ctx, s.manifestObjects[manifest]...)
	}
}

func (s *tsuite) AfterTest(suiteName, testName string) {
	manifests := s.manifests[testName]
	for _, manifest := range manifests {
		err := s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.Require().NoError(err)
		s.ti.Assertions.EventuallyObjectsNotExist(s.ctx, s.manifestObjects[manifest]...)
	}
}

func (s *tsuite) TestBasic() {
	// Assert traffic to team1 route
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt, []curl.Option{curl.WithHostPort(proxyHostPort), curl.WithPath(pathTeam1)},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam1)})

	// Assert traffic to team2 route
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt, []curl.Option{curl.WithHostPort(proxyHostPort), curl.WithPath(pathTeam2)},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam2)})
}

func (s *tsuite) TestRecursive() {
	// Assert traffic to team1 route
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt, []curl.Option{curl.WithHostPort(proxyHostPort), curl.WithPath(pathTeam1)},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam1)})

	// Assert traffic to team2 route
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt, []curl.Option{curl.WithHostPort(proxyHostPort), curl.WithPath(pathTeam2)},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam2)})
}

func (s *tsuite) TestCyclic() {
	// Assert traffic to team1 route
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt, []curl.Option{curl.WithHostPort(proxyHostPort), curl.WithPath(pathTeam1)},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam1)})

	// Assert traffic to team2 route fails with HTTP 500 as it is a cyclic route
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt, []curl.Option{curl.WithHostPort(proxyHostPort), curl.WithPath(pathTeam2)},
		&testmatchers.HttpResponse{StatusCode: http.StatusInternalServerError})

	s.ti.Assertions.EventuallyHTTPRouteStatusContainsMessage(s.ctx, routeTeam2.Name, routeTeam2.Namespace,
		"cyclic reference detected", 10*time.Second, 1*time.Second)
}

func (s *tsuite) TestInvalidChild() {
	// Assert traffic to team1 route
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt, []curl.Option{curl.WithHostPort(proxyHostPort), curl.WithPath(pathTeam1)},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam1)})

	// Assert traffic to team2 route fails with HTTP 500 as the route is invalid due to specifying a hostname on the child route
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt, []curl.Option{curl.WithHostPort(proxyHostPort), curl.WithPath(pathTeam2)},
		&testmatchers.HttpResponse{StatusCode: http.StatusInternalServerError})

	s.ti.Assertions.EventuallyHTTPRouteStatusContainsMessage(s.ctx, routeTeam2.Name, routeTeam2.Namespace,
		"spec.hostnames must be unset", 10*time.Second, 1*time.Second)
}

func (s *tsuite) TestHeaderQueryMatch() {
	// Assert traffic to team1 route with matching header and query parameters
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHostPort(proxyHostPort), curl.WithPath(pathTeam1),
			curl.WithHeader("header1", "val1"),
			curl.WithHeader("headerX", "valX"),
			curl.WithQueryParameters(map[string]string{"query1": "val1", "queryX": "valX"}),
		},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam1)})

	// Assert traffic to team2 child route fails with HTTP 404 as it does not match the parent's header and query parameters
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHostPort(proxyHostPort), curl.WithPath(pathTeam2),
			curl.WithHeader("headerX", "valX"),
			curl.WithQueryParameters(map[string]string{"queryX": "valX"}),
		},
		&testmatchers.HttpResponse{StatusCode: http.StatusNotFound})

	// Assert traffic to team2 parent route fails with HTTP 500 due to unresolved child route
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHostPort(proxyHostPort), curl.WithPath(pathTeam2),
			curl.WithHeader("header2", "val2"),
			curl.WithQueryParameters(map[string]string{"query2": "val2"}),
		},
		&testmatchers.HttpResponse{StatusCode: http.StatusInternalServerError})
}

func (s *tsuite) TestMultipleParents() {
	// Assert traffic to parent1.com/anything/team1
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHostPort(proxyHostPort),
			curl.WithPath(pathTeam1),
			curl.WithHostHeader(routeParent1Host),
		},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam1)})

	// Assert traffic to parent1.com/anything/team2
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHostPort(proxyHostPort),
			curl.WithPath(pathTeam2),
			curl.WithHostHeader(routeParent1Host),
		},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam2)})

	// Assert traffic to parent2.com/anything/team1
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHostPort(proxyHostPort),
			curl.WithPath(pathTeam1),
			curl.WithHostHeader(routeParent2Host),
		},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam1)})

	// Assert traffic to parent2.com/anything/team2 fails as it is not selected by parent2 route
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHostPort(proxyHostPort),
			curl.WithPath(pathTeam2),
			curl.WithHostHeader(routeParent2Host),
		},
		&testmatchers.HttpResponse{StatusCode: http.StatusInternalServerError})
}

func (s *tsuite) TestInvalidChildValidStandalone() {
	// Assert traffic to team1 route
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHostPort(proxyTestHostPort),
			curl.WithPath(pathTeam1),
			curl.WithHostHeader(routeParentHost),
		},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam1)})

	// Assert traffic to team2 route on parent hostname fails with HTTP 500 as the route is invalid due to specifying a hostname on the child route
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHostPort(proxyTestHostPort),
			curl.WithPath(pathTeam2),
			curl.WithHostHeader(routeParentHost),
		},
		&testmatchers.HttpResponse{StatusCode: http.StatusInternalServerError})

	// Assert traffic to team2 route on standalone host succeeds
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHostPort(proxyTestHostPort),
			curl.WithPath(pathTeam2),
			curl.WithHostHeader(routeTeam2Host),
		},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring(pathTeam2)})

	s.ti.Assertions.EventuallyHTTPRouteStatusContainsMessage(s.ctx, routeTeam2.Name, routeTeam2.Namespace,
		"spec.hostnames must be unset", 10*time.Second, 1*time.Second)
}

func (s *tsuite) TestUnresolvedChild() {
	s.ti.Assertions.EventuallyHTTPRouteStatusContainsReason(s.ctx, routeRoot.Name, routeRoot.Namespace,
		string(gwv1.RouteReasonBackendNotFound), 10*time.Second, 1*time.Second)
}

func (s *tsuite) TestMatcherInheritance() {
	// Wait for both parent routes to be accepted before sending traffic
	s.ti.Assertions.EventuallyHTTPRouteStatusContainsReason(s.ctx, routeParent1.Name, routeParent1.Namespace,
		string(gwv1.RouteReasonAccepted), 10*time.Second, 1*time.Second)
	s.ti.Assertions.EventuallyHTTPRouteStatusContainsReason(s.ctx, routeParent2.Name, routeParent2.Namespace,
		string(gwv1.RouteReasonAccepted), 10*time.Second, 1*time.Second)
	s.ti.Assertions.EventuallyHTTPRouteStatusContainsReason(s.ctx, routeTeam1.Name, routeTeam1.Namespace,
		string(gwv1.RouteReasonAccepted), 10*time.Second, 1*time.Second)

	// Assert traffic on parent1's prefix
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{curl.WithHostPort(proxyHostPort), curl.WithPath("/anything/foo/child")},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring("/anything/foo/child")})

	// Assert traffic on parent2's prefix
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{curl.WithHostPort(proxyHostPort), curl.WithPath("/anything/baz/child")},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Body: ContainSubstring("/anything/baz/child")})
}

func (s *tsuite) TestRouteWeight() {
	// Assert traffic to /anything path prefix is always routed to svc1
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{curl.WithHostPort(proxyHostPort), curl.WithPath(pathTeam1)},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       ContainSubstring(pathTeam1),
			Headers: map[string]any{
				"origin": "svc1",
			},
		})
	// Assert traffic to /anything/team2 is also routed to svc1 since its route has higher weight
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{curl.WithHostPort(proxyHostPort), curl.WithPath(pathTeam2)},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       ContainSubstring(pathTeam2),
			Headers: map[string]any{
				"origin": "svc1",
			},
		})
}

func (s *tsuite) TestPolicyMerging() {
	// Assert traffic to parent1.com/anything/team1 uses svc1's transformation policy
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHostPort(proxyHostPort),
			curl.WithPath(pathTeam1),
			curl.WithHostHeader(routeParent1Host),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       ContainSubstring(pathTeam1),
			Headers: map[string]any{
				"origin": "svc1",
			},
		})

	// Assert traffic to parent1.com/anything/team2 uses svc2's transformation policy
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHostPort(proxyHostPort),
			curl.WithPath(pathTeam2),
			curl.WithHostHeader(routeParent1Host),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       ContainSubstring(pathTeam2),
			Headers: map[string]any{
				"origin": "svc2",
			},
		})

	// Assert traffic to parent2.com/anything/team1 uses parent2's transformation policy
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHostPort(proxyHostPort),
			curl.WithPath(pathTeam1),
			curl.WithHostHeader(routeParent2Host),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       ContainSubstring(pathTeam1),
			Headers: map[string]any{
				"origin": "parent2",
			},
		})

	// Assert traffic to parent2.com/anything/team2 uses parent2's transformation policy
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHostPort(proxyHostPort),
			curl.WithPath(pathTeam2),
			curl.WithHostHeader(routeParent2Host),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       ContainSubstring(pathTeam2),
			Headers: map[string]any{
				"origin": "parent2",
			},
		})
}
