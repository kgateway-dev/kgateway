package http_listener_policy

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is the entire Suite of tests for the "HttpListenerPolicy" feature
type testingSuite struct {
	suite.Suite
	ctx              context.Context
	testInstallation *e2e.TestInstallation
	// maps test name to a list of manifests to apply before the test
	manifests map[string][]string
}

func NewTestingSuite(
	ctx context.Context,
	testInst *e2e.TestInstallation,
) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	// Check that the common setup manifest is applied
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.NoError(err, "can apply "+setupManifest)
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, exampleSvc, nginxPod)
	// Check that test app is running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, nginxPod.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=nginx",
	})

	// include gateway manifests for the tests, so we recreate it for each test run
	s.manifests = map[string][]string{
		"TestHttpListenerPolicyAllFields":    {gatewayManifest, httpRouteManifest, allFieldsManifest},
		"TestHttpListenerPolicyServerHeader": {gatewayManifest, httpRouteManifest, serverHeaderManifest},
		"TestPreserveHttp1HeaderCase":        {gatewayManifest, preserveHttp1HeaderCaseManifest},
		"TestAccessLogEmittedToStdout":       {gatewayManifest, httpRouteManifest, accessLogManifest},
	}
}

func (s *testingSuite) TearDownSuite() {
	// Check that the common setup manifest is deleted
	err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, setupManifest)
	s.NoError(err, "can delete "+setupManifest)
}

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for %s, manifest map contents: %v", testName, s.manifests)
	}

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Assert().NoError(err, "can apply manifest "+manifest)
	}

	// we recreate the `Gateway` resource (and thus dynamically provision the proxy pod) for each test run
	// so let's assert the proxy svc and pod is ready before moving on
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gw",
	})
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for " + testName)
	}

	for _, manifest := range manifests {
		output, err := s.testInstallation.Actions.Kubectl().DeleteFileWithOutput(s.ctx, manifest)
		s.testInstallation.Assertions.ExpectObjectDeleted(manifest, err, output)
	}
}

func (s *testingSuite) TestHttpListenerPolicyAllFields() {
	// Test that the HTTPListenerPolicy with all additional fields is applied correctly
	// The test verifies that the gateway is working and all policy fields are applied
	fmt.Println("TestHttpListenerPolicyAllFields")
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring("Welcome to nginx!"),
		})

	// Check the health check path is working
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPath("/health_check"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.BeEmpty(),
		})
}

func (s *testingSuite) TestHttpListenerPolicyServerHeader() {
	// Test that the HTTPListenerPolicy with serverHeaderTransformation field is applied correctly
	// The test verifies that the server header is transformed as expected
	// With PassThrough, the server header should be the backend server's header (nginx/1.28.0)
	// instead of Envoy's default (envoy)
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring("Welcome to nginx!"),
			Headers: map[string]any{
				"server": "nginx/1.28.0", // Should be the backend server header, not "envoy"
			},
		})
}

func (s *testingSuite) TestPreserveHttp1HeaderCase() {
	// The test verifies that the HTTP1 headers are preserved as expected in the request and response
	// The HTTPListenerPolicy ensures that the header is preserved in the request,
	// and the BackendConfigPolicy ensures that the header is preserved in the response.
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, echoService, echoDeployment)
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, echoDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=raw-header-echo",
	})
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithHeader("X-CaSeD-HeAdEr", "test"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring("X-CaSeD-HeAdEr"),
			Headers: map[string]any{
				"ReSpOnSe-miXed-CaSe-hEaDeR": "Foo",
			},
		},
	)
}

func (s *testingSuite) TestAccessLogEmittedToStdout() {
	// First: trigger a 404 that SHOULD be logged (filter is GE 400)
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("not.example.com"), // not matched by HTTPRoute hostnames
			curl.WithPath("/does-not-exist"),
		},
		&matchers.HttpResponse{StatusCode: http.StatusNotFound},
	)

	// Fetch gateway pod logs and verify the 404 access log JSON fields are present
	pods, err := s.testInstallation.Actions.Kubectl().GetPodsInNsWithLabel(
		s.ctx, proxyDeployment.ObjectMeta.GetNamespace(),
		"app.kubernetes.io/name="+proxyDeployment.ObjectMeta.GetName(),
	)
	s.Require().NoError(err)
	s.Require().Len(pods, 1)

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.testInstallation.Actions.Kubectl().GetContainerLogs(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(), pods[0])
		s.Require().NoError(err)
		// Check a few key fields configured in http-listener-policy-access-log.yaml jsonFormat
		assert.Contains(c, logs, "\"method\":\"GET\"")
		assert.Contains(c, logs, "\"protocol\":\"HTTP/1.1\"")
		assert.Contains(c, logs, "\"response_code\":404")
		assert.Contains(c, logs, "\"path\":\"/does-not-exist\"")
	}, 30*time.Second, 200*time.Millisecond)

	// Second: trigger a 200 that SHOULD NOT be logged due to filter GE 400
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPath("/"),
		},
		&matchers.HttpResponse{StatusCode: http.StatusOK},
	)

	// Confirm 200 logs do not appear over a stability window as it isn't being immediately emitted
	g := gomega.NewWithT(s.T())
	g.Consistently(func() string {
		out, err := s.testInstallation.Actions.Kubectl().GetContainerLogs(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(), pods[0])
		s.Require().NoError(err)
		return out
	}, 10*time.Second, 200*time.Millisecond).ShouldNot(gomega.ContainSubstring("\"response_code\":200"))
}
