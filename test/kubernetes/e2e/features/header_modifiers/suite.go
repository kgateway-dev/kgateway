package header_modifiers

import (
	"context"
	"encoding/json"
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
		testdefaults.CurlPodManifest,
		testdefaults.HttpbinManifest,
		commonManifest,
	}
	s.commonResources = []client.Object{
		testdefaults.CurlPod,
		testdefaults.HttpbinDeployment, httpbinSvc,
		gateway, route1,
		listenerSet, route2,
		proxyDeployment, proxyService, proxyServiceAccount,
	}

	// set up common resources once
	for _, manifest := range s.commonManifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err, "can apply "+manifest)
	}
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, s.commonResources...)

	// make sure pods are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx,
		testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
			LabelSelector: testdefaults.CurlPodLabelSelector,
		})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx,
		testdefaults.HttpbinDeployment.GetNamespace(), metav1.ListOptions{
			LabelSelector: testdefaults.HttpbinLabelSelector,
		})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx,
		proxyObjectMeta.GetNamespace(), metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", proxyObjectMeta.GetName()),
		})
}

func (s *testingSuite) TearDownSuite() {
	// clean up common resources
	for _, manifest := range s.commonManifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.Require().NoError(err, "can delete "+manifest)
	}
	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, s.commonResources...)

	// make sure pods are gone
	s.testInstallation.Assertions.EventuallyPodsNotExist(s.ctx,
		testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
			LabelSelector: testdefaults.CurlPodLabelSelector,
		})
	s.testInstallation.Assertions.EventuallyPodsNotExist(s.ctx,
		testdefaults.HttpbinDeployment.GetNamespace(), metav1.ListOptions{
			LabelSelector: testdefaults.HttpbinLabelSelector,
		})
	s.testInstallation.Assertions.EventuallyPodsNotExist(s.ctx,
		proxyObjectMeta.GetNamespace(), metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", proxyObjectMeta.GetName()),
		})
}

func (s *testingSuite) TestRouteLevelHeaderModifiers() {
	s.setupTest([]string{headerModifiersRouteTrafficPolicyManifest},
		[]client.Object{routeTrafficPolicy1})

	s.assertHeaders(8080, expectedRequestHeaders("route"), expectedResponseHeaders("route"))
}

func (s *testingSuite) TestGatewayLevelHeaderModifiers() {
	s.setupTest([]string{headerModifiersGwTrafficPolicyManifest},
		[]client.Object{gwtrafficPolicy})

	s.assertHeaders(8080, expectedRequestHeaders("gw"), expectedResponseHeaders("gw"))
}

func (s *testingSuite) TestListenerSetLevelHeaderModifiers() {
	s.setupTest([]string{headerModifiersLsTrafficPolicyManifest},
		[]client.Object{lsTrafficPolicy})

	s.assertHeaders(8081, expectedRequestHeaders("ls"), expectedResponseHeaders("ls"))
}

func (s *testingSuite) TestMultiLevelHeaderModifiers() {
	s.setupTest([]string{
		headerModifiersGwTrafficPolicyManifest,
		headerModifiersLsTrafficPolicyManifest,
		headerModifiersRouteTrafficPolicyManifest,
		headerModifiersRouteListenerSetTrafficPolicyManifest,
	}, []client.Object{
		gwtrafficPolicy,
		lsTrafficPolicy,
		routeTrafficPolicy1,
		routeTrafficPolicy2,
	})

	s.assertHeaders(8080, expectedRequestHeaders("route", "gw"), nil)
	s.assertHeaders(8081, expectedRequestHeaders("route", "ls", "gw"), nil)
}

func expectedRequestHeaders(suffixes ...string) map[string][]any {
	h := map[string][]any{}

	for _, suffix := range suffixes {
		h["X-Custom-Request-Header"] = append(h["X-Custom-Request-Header"],
			"custom-request-value-"+suffix)
	}

	if len(suffixes) > 0 {
		h["X-Custom-Request-Header-Set"] = []any{
			"custom-request-value-" + suffixes[len(suffixes)-1]}
	}

	return h
}

func expectedResponseHeaders(suffix string) map[string]any {
	return map[string]any{
		"X-Custom-Response-Header":     "custom-response-value-" + suffix,
		"X-Custom-Response-Header-Set": "custom-response-value-" + suffix,
	}
}

func (s *testingSuite) setupTest(manifests []string, resources []client.Object) {
	s.T().Cleanup(func() {
		for _, manifest := range manifests {
			err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
			s.Require().NoError(err)
		}

		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, resources...)
	})

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err, "can apply "+manifest)
	}

	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, resources...)
}

func (s *testingSuite) assertHeaders(port int,
	requestHeaders map[string][]any,
	responseHeaders map[string]any,
) {
	allOptions := []curl.Option{
		curl.WithPath("/headers"),
		curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
		curl.WithHostHeader("example.com"),
		curl.WithPort(port),
	}

	requestHeadersJSON, err := json.Marshal(map[string]any{"headers": requestHeaders})
	s.Require().NoError(err, "unable to marshal request headers to JSON")

	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		allOptions,
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Headers:    responseHeaders,
			NotHeaders: []string{"X-Request-Id", "X-Envoy-Upstream-Service-Time"},
			Body:       testmatchers.JSONContains(requestHeadersJSON),
		},
	)
}
