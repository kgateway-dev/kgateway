package session_persistence

import (
	"context"
	"fmt"
	"strings"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

type testingSuite struct {
	suite.Suite

	ctx context.Context

	testInstallation *e2e.TestInstallation
	manifests        map[string][]string
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.CurlPodManifest)
	s.NoError(err, "can apply curl pod manifest")

	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, testdefaults.CurlPod)
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, echoServiceManifest)
	s.NoError(err, "can apply echo service manifest")

	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, echoService, echoDeployment)
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, echoDeployment.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=echo",
	})

	s.manifests = map[string][]string{
		"TestCookieSessionPersistence": {cookieSessionPersistenceManifest},
		"TestHeaderSessionPersistence": {headerSessionPersistenceManifest},
	}
}

func (s *testingSuite) TearDownSuite() {
	err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, testdefaults.CurlPodManifest)
	s.NoError(err, "can delete curl pod manifest")

	err = s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, echoServiceManifest)
	s.NoError(err, "can delete echo service manifest")
}

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for %s, manifest map contents: %v", testName, s.manifests)
	}

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err, "can apply manifest "+manifest)
	}

	switch testName {
	case "TestCookieSessionPersistence":
		s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, cookieGateway, cookieHTTPRoute)
		s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, cookieGateway.GetNamespace(), metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", cookieGateway.Name),
		})
	case "TestHeaderSessionPersistence":
		s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, headerGateway, headerHTTPRoute)
		s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, headerGateway.GetNamespace(), metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", headerGateway.Name),
		})
	}
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for %s, manifest map contents: %v", testName, s.manifests)
	}

	// Clean up test-specific manifests
	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, manifest, "--grace-period", "0")
		s.NoError(err, "can delete manifest "+manifest)
	}
}

func (s *testingSuite) TestCookieSessionPersistence() {
	s.assertSessionPersistence("cookie")
}

func (s *testingSuite) TestHeaderSessionPersistence() {
	s.assertSessionPersistence("header")
}

// assertSessionPersistence makes multiple requests and verifies they go to the same backend pod
func (s *testingSuite) assertSessionPersistence(persistenceType string) {
	var (
		gatewayService metav1.ObjectMeta
		sessionHeader  string
	)

	switch persistenceType {
	case "cookie":
		gatewayService = cookieGateway.ObjectMeta
	case "header":
		gatewayService = headerGateway.ObjectMeta
		sessionHeader = "session-a"
	}

	firstCurlOpts := []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(gatewayService)),
		curl.WithHostHeader("echo.local"),
		curl.WithPort(8080),
		curl.Silent(),
		curl.WithArgs([]string{"-i"}),
	}

	firstResp, err := s.testInstallation.ClusterContext.Cli.CurlFromPod(s.ctx, testdefaults.CurlPodExecOpt, firstCurlOpts...)
	s.Assert().NoError(err, "first request should succeed")

	firstPodName := s.extractPodNameFromResponse(firstResp.StdOut)
	s.Assert().NotEmpty(firstPodName, "should be able to extract pod name from first response")

	var subsequentCurlOpts []curl.Option
	if persistenceType == "cookie" {
		cookie := s.extractSessionCookieFromResponse(firstResp.StdOut)
		s.Assert().NotEmpty(cookie, "should have received a session cookie")
		subsequentCurlOpts = []curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService)),
			curl.WithHostHeader("echo.local"),
			curl.WithPort(8080),
			curl.Silent(),
			curl.WithHeader("Cookie", cookie),
		}
	} else {
		headerValue := s.extractSessionHeaderFromResponse(firstResp.StdOut)
		s.Assert().NotEmpty(headerValue, "should have received a session header")
		subsequentCurlOpts = []curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService)),
			curl.WithHostHeader("echo.local"),
			curl.WithPort(8080),
			curl.Silent(),
			curl.WithHeader(sessionHeader, headerValue),
		}
	}

	for i := 0; i < 10; i++ {
		resp, err := s.testInstallation.ClusterContext.Cli.CurlFromPod(s.ctx, testdefaults.CurlPodExecOpt, subsequentCurlOpts...)
		s.Assert().NoError(err, fmt.Sprintf("request %d should succeed", i+2))

		podName := s.extractPodNameFromResponse(resp.StdOut)
		s.Assert().Equal(firstPodName, podName, fmt.Sprintf("request %d should go to the same pod due to session persistence", i+2))
	}
}

// extractPodNameFromResponse extracts the pod name from the echo service response
func (s *testingSuite) extractPodNameFromResponse(response string) string {
	// The echo service returns something like "pod=echo-abc123-xyz"
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		if strings.Contains(line, "pod=") {
			parts := strings.Split(line, "pod=")
			if len(parts) > 1 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// extractSessionCookieFromResponse extracts the session cookie from the curl response
func (s *testingSuite) extractSessionCookieFromResponse(response string) string {
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		if strings.Contains(line, "set-cookie:") && strings.Contains(line, "Session-A") {
			// Extract cookie value from "Set-Cookie: Session-A=value; ..."
			parts := strings.Split(line, "set-cookie:")
			if len(parts) > 1 {
				cookiePart := strings.TrimSpace(parts[1])
				// Take only the cookie name=value part (before any semicolon)
				if idx := strings.Index(cookiePart, ";"); idx != -1 {
					cookiePart = cookiePart[:idx]
				}
				return strings.TrimSpace(cookiePart)
			}
		}
	}
	return ""
}

// extractSessionHeaderFromResponse extracts the session header from the curl response
func (s *testingSuite) extractSessionHeaderFromResponse(response string) string {
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		if strings.Contains(line, "session-a:") {
			parts := strings.Split(line, "session-a:")
			if len(parts) > 1 {
				cookiePart := strings.TrimSpace(parts[1])
				if idx := strings.Index(cookiePart, ";"); idx != -1 {
					cookiePart = cookiePart[:idx]
				}
				return strings.TrimSpace(cookiePart)
			}
		}
	}
	return ""
}
