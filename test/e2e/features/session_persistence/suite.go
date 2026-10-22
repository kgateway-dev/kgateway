//go:build e2e

package session_persistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

var (
	setup = base.TestCase{
		Manifests: []string{
			testdefaults.CurlPodManifest,
			echoServiceManifest,
		},
	}

	testCases = map[string]*base.TestCase{
		"TestCookieSessionPersistence": {
			Manifests: []string{cookieSessionPersistenceManifest},
		},
		"TestHeaderSessionPersistence": {
			Manifests: []string{headerSessionPersistenceManifest},
		},
		"TestBackendConfigPolicyCookieSessionPersistence": {
			Manifests: []string{bcpCookieSessionPersistenceManifest},
		},
		"TestBackendConfigPolicyHeaderSessionPersistence": {
			Manifests: []string{bcpHeaderSessionPersistenceManifest},
		},
		"TestBackendConfigPolicySessionPersistenceDynamicUpdate": {
			Manifests: []string{bcpDynamicRouteManifest},
		},
	}
)

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases,
			base.WithMinGwApiVersion(base.GwApiRequireSessionPersistence),
		),
	}
}

func (s *testingSuite) TestCookieSessionPersistence() {
	s.assertSessionPersistence("gw-cookie", "cookie")
}

func (s *testingSuite) TestHeaderSessionPersistence() {
	s.assertSessionPersistence("gw-header", "header")
}

func (s *testingSuite) TestBackendConfigPolicyCookieSessionPersistence() {
	s.assertSessionPersistence("gw-bcp-cookie", "cookie")
}

func (s *testingSuite) TestBackendConfigPolicyHeaderSessionPersistence() {
	s.assertSessionPersistence("gw-bcp-header", "header")
}

// TestBackendConfigPolicySessionPersistenceDynamicUpdate verifies that adding and removing a
// BackendConfigPolicy on a Service that an existing route already uses updates the route config.
func (s *testingSuite) TestBackendConfigPolicySessionPersistenceDynamicUpdate() {
	const gatewayName = "gw-bcp-dynamic"

	s.T().Log("Phase 1: route without a policy does not set a session cookie")
	s.eventuallySessionCookie(gatewayName, false)

	s.T().Log("Phase 2: adding the policy enables session persistence on the existing route")
	policyApplied := true
	testutils.Cleanup(s.T(), func() {
		if policyApplied {
			s.DeleteManifests(&base.TestCase{Manifests: []string{bcpDynamicPolicyManifest}})
		}
	})
	s.ApplyManifests(&base.TestCase{Manifests: []string{bcpDynamicPolicyManifest}})
	s.eventuallySessionCookie(gatewayName, true)
	s.assertSessionPersistence(gatewayName, "cookie")

	s.T().Log("Phase 3: removing the policy disables session persistence again")
	s.DeleteManifests(&base.TestCase{Manifests: []string{bcpDynamicPolicyManifest}})
	policyApplied = false
	s.eventuallySessionCookie(gatewayName, false)
}

// eventuallySessionCookie waits until responses from the gateway do (or do not) set the session cookie.
func (s *testingSuite) eventuallySessionCookie(gatewayName string, want bool) {
	gatewayService := metav1.ObjectMeta{Name: gatewayName, Namespace: "default"}
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		resp, err := s.TestInstallation.ClusterContext.Cli.CurlFromPod(s.Ctx, testdefaults.CurlPodExecOpt,
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService)),
			curl.WithHostHeader("echo.local"),
			curl.WithPort(8080),
			curl.Silent(),
			curl.WithArgs([]string{"-i"}),
			curl.WithConnectionTimeout(10),
		)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "request should succeed")
		g.Expect(s.extractPodNameFromResponse(resp.StdOut)).NotTo(gomega.BeEmpty(), "request should reach the echo backend")
		if want {
			g.Expect(s.extractSessionCookieFromResponse(resp.StdOut)).NotTo(gomega.BeEmpty(), "response should set the session cookie")
		} else {
			g.Expect(s.extractSessionCookieFromResponse(resp.StdOut)).To(gomega.BeEmpty(), "response should not set the session cookie")
		}
	}).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(gomega.Succeed())
}

// assertSessionPersistence makes multiple requests through the given gateway and verifies they go to the same backend pod
func (s *testingSuite) assertSessionPersistence(gatewayName, persistenceType string) {
	gatewayService := metav1.ObjectMeta{
		Name:      gatewayName,
		Namespace: "default",
	}

	firstCurlOpts := []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(gatewayService)),
		curl.WithHostHeader("echo.local"),
		curl.WithPort(8080),
		curl.Silent(),
		curl.WithArgs([]string{"-i"}),
		curl.WithConnectionTimeout(10),
		curl.WithRetries(3, 0, 10),
		curl.WithRetryConnectionRefused(true),
	}

	firstResp, err := s.TestInstallation.ClusterContext.Cli.CurlFromPod(s.Ctx, testdefaults.CurlPodExecOpt, firstCurlOpts...)
	s.Require().NoError(err, "first request should succeed")

	firstPodName := s.extractPodNameFromResponse(firstResp.StdOut)
	s.Require().NotEmpty(firstPodName, "should be able to extract pod name from first response")

	var subsequentCurlOpts []curl.Option
	if persistenceType == "cookie" {
		cookie := s.extractSessionCookieFromResponse(firstResp.StdOut)
		s.Require().NotEmpty(cookie, "should have received a session cookie")
		subsequentCurlOpts = []curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService)),
			curl.WithHostHeader("echo.local"),
			curl.WithPort(8080),
			curl.Silent(),
			curl.WithHeader("Cookie", cookie),
			curl.WithConnectionTimeout(10),
			curl.WithRetries(5, 0, 10),
			curl.WithRetryConnectionRefused(true),
		}
	} else {
		headerValue := s.extractSessionHeaderFromResponse(firstResp.StdOut)
		s.Require().NotEmpty(headerValue, "should have received a session header")
		subsequentCurlOpts = []curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService)),
			curl.WithHostHeader("echo.local"),
			curl.WithPort(8080),
			curl.Silent(),
			curl.WithHeader("session-a", headerValue),
			curl.WithConnectionTimeout(10),
			curl.WithRetries(3, 0, 10),
			curl.WithRetryConnectionRefused(true),
		}
	}

	for i := range 10 {
		resp, err := s.TestInstallation.ClusterContext.Cli.CurlFromPod(s.Ctx, testdefaults.CurlPodExecOpt, subsequentCurlOpts...)
		s.Assert().NoError(err, fmt.Sprintf("request %d should succeed", i+2))

		podName := s.extractPodNameFromResponse(resp.StdOut)
		s.Assert().Equal(firstPodName, podName, fmt.Sprintf("request %d should go to the same pod due to session persistence", i+2))
	}
}

// extractPodNameFromResponse extracts the pod name from the echo service response
func (s *testingSuite) extractPodNameFromResponse(response string) string {
	// The echo service returns something like "pod=echo-abc123-xyz"
	lines := strings.SplitSeq(response, "\n")
	for line := range lines {
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
	lines := strings.SplitSeq(response, "\n")
	for line := range lines {
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
	lines := strings.SplitSeq(response, "\n")
	for line := range lines {
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
