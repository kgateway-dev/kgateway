//go:build e2e

package http_acl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	adminv3 "github.com/envoyproxy/go-control-plane/envoy/admin/v3"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/encoding/protojson"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/envoyutils/admincli"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of tests for the HTTP ACL filter functionality.
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

// Note: useRemoteAddress is explicitly set to false in setup.yaml so we can use the
// x-forwarded-for header to fake the client ip for testing

// TestHttpACLDefaultAllowDenyCIDR tests defaultAction=allow with a deny rule for 192.168.0.0/16.
// Requests from 10.0.0.1 pass; requests from 192.168.1.100 are blocked with 403.
func (s *testingSuite) TestHttpACLDefaultAllowDenyCIDR() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx, "httpbin-route", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue,
	)

	s.T().Log("IP outside blocked CIDR should be allowed")
	common.BaseGateway.Send(
		s.T(),
		expectAllowed,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "10.0.0.1"),
	)

	s.T().Log("IP inside blocked CIDR 192.168.0.0/16 should be denied")
	common.BaseGateway.Send(
		s.T(),
		expectDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "192.168.1.100"),
	)
}

// TestHttpACLDefaultDenyAllowSubnet tests defaultAction=deny with an allow rule for 10.0.0.0/8.
// Requests from 10.5.5.5 pass; requests from 8.8.8.8 are blocked with 403.
func (s *testingSuite) TestHttpACLDefaultDenyAllowSubnet() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx, "httpbin-route", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue,
	)

	s.T().Log("IP inside allowed subnet 10.0.0.0/8 should be allowed")
	common.BaseGateway.Send(
		s.T(),
		expectAllowed,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "10.5.5.5"),
	)

	s.T().Log("IP outside allowed subnet should be denied by defaultAction=deny")
	common.BaseGateway.Send(
		s.T(),
		expectDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "8.8.8.8"),
	)
}

// TestHttpACLHolePunchNamedRules tests longest-prefix matching with overlapping rules:
// defaultAction=allow, block 10.0.0.0/8, allow 10.1.0.0/16, block 10.1.2.3/32.
func (s *testingSuite) TestHttpACLHolePunchNamedRules() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx, "httpbin-route", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue,
	)

	s.T().Log("10.1.2.3 matches block-rogue-host (/32 beats /8 and /16) → denied")
	common.BaseGateway.Send(
		s.T(),
		expectDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "10.1.2.3"),
	)

	s.T().Log("10.1.2.4 matches allow-trusted-subnet (/16 beats /8) → allowed")
	common.BaseGateway.Send(
		s.T(),
		expectAllowed,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "10.1.2.4"),
	)

	s.T().Log("10.2.0.1 matches block-internal-range (/8) → denied")
	common.BaseGateway.Send(
		s.T(),
		expectDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "10.2.0.1"),
	)

	s.T().Log("8.8.8.8 matches no rule → allowed by defaultAction=allow")
	common.BaseGateway.Send(
		s.T(),
		expectAllowed,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "8.8.8.8"),
	)
}

// TestHttpACLCustomDenyResponse tests a custom deny status code (451) and extra response headers.
func (s *testingSuite) TestHttpACLCustomDenyResponse() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx, "httpbin-route", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue,
	)

	s.T().Log("Denied request should return 451 with custom headers X-Blocked-Reason and Retry-After")
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusUnavailableForLegalReasons,
			Headers: map[string]any{
				"X-Blocked-Reason": "geo-policy",
				"Retry-After":      "3600",
			},
		},
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "10.0.0.1"),
	)
}

// TestHttpACLBlockedByHeader tests that blockedByHeaderName surfaces the block reason in a response header.
// Rules: deny 10.0.0.0/8 (named), deny 192.168.0.0/16 (unnamed), allow 203.0.113.0/24; defaultAction=deny.
func (s *testingSuite) TestHttpACLBlockedByHeader() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx, "httpbin-route", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue,
	)

	s.T().Log("10.5.5.5 matches named rule block-internal-range → X-Blocked-By: block-internal-range")
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusForbidden,
			Headers:    map[string]any{"X-Blocked-By": "block-internal-range"},
		},
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "10.5.5.5"),
	)

	s.T().Log("192.168.1.1 matches unnamed rule → X-Blocked-By: rule")
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusForbidden,
			Headers:    map[string]any{"X-Blocked-By": "rule"},
		},
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "192.168.1.1"),
	)

	s.T().Log("8.8.8.8 matches no rule, defaultAction=deny → X-Blocked-By: default")
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusForbidden,
			Headers:    map[string]any{"X-Blocked-By": "default"},
		},
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "8.8.8.8"),
	)

	s.T().Log("203.0.113.5 matches allow rule → 200 OK, no X-Blocked-By header")
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			NotHeaders: []string{"X-Blocked-By"},
		},
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "203.0.113.5"),
	)
}

// TestHttpACLBlockedCounter verifies that the Envoy counter dev.kgateway.http.acl.blocked
// increments when requests are denied.
func (s *testingSuite) TestHttpACLBlockedCounter() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx, "httpbin-route", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue,
	)

	// Send several denied requests to ensure the counter increments.
	for range 3 {
		common.BaseGateway.Send(
			s.T(),
			expectDenied,
			curl.WithHostHeader("httpbin"),
			curl.WithPort(80),
			curl.WithPath("/status/200"),
			curl.WithHeader("X-Forwarded-For", "8.8.8.8"),
		)
	}

	// Verify the blocked counter increased via the Envoy admin /stats endpoint.
	s.TestInstallation.AssertionsT(s.T()).AssertEnvoyAdminApi(
		s.Ctx,
		proxyObjectMeta,
		func(ctx context.Context, adminClient *admincli.Client) {
			s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
				// The counter name is dev.kgateway.http.acl.blocked; Envoy may prepend
				// a dynamic-modules stats scope prefix, so match on the suffix.
				counterSuffix := "dev.kgateway.http.acl.blocked"
				out, err := adminClient.GetStats(ctx, map[string]string{
					"format": "json",
					"filter": ".*" + strings.ReplaceAll(counterSuffix, ".", "\\.") + "$",
				})
				g.Expect(err).NotTo(gomega.HaveOccurred(), "can get envoy stats")

				var resp map[string][]adminv3.SimpleMetric
				g.Expect(json.Unmarshal([]byte(out), &resp)).To(gomega.Succeed(), "can unmarshal envoy stats")

				stats := resp["stats"]
				g.Expect(stats).NotTo(gomega.BeEmpty(), "expected at least one matching stat for %s", counterSuffix)
				g.Expect(stats[0].GetValue()).To(gomega.BeNumerically(">=", 1.0),
					"blocked counter should be at least 1 after denied requests")
			}).WithTimeout(10 * time.Second).WithPolling(time.Second).
				Should(gomega.Succeed())
		},
	)
}

// TestHttpACLRouteLevel verifies that an ACL policy attached via ExtensionRef applies
// only to the specific route rule it references, leaving other rules unfiltered.
// The /status route denies 192.168.0.0/16; the /get route has no ACL at all.
func (s *testingSuite) TestHttpACLRouteLevel() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx, "httpbin-route", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue,
	)

	s.T().Log("/status route: blocked IP should be denied by per-rule ACL")
	common.BaseGateway.Send(
		s.T(),
		expectDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "192.168.1.100"),
	)

	s.T().Log("/status route: allowed IP should pass")
	common.BaseGateway.Send(
		s.T(),
		expectAllowed,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "10.0.0.1"),
	)

	s.T().Log("/get route: same blocked IP should pass (ACL not attached to this rule)")
	common.BaseGateway.Send(
		s.T(),
		expectAllowed,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
		curl.WithHeader("X-Forwarded-For", "192.168.1.100"),
	)
}

// TestHttpACLHTTPRouteLevel verifies that an ACL policy attached via targetRef to an HTTPRoute
// applies to all route rules within that HTTPRoute.
// defaultAction=deny, allow 10.0.0.0/8 — both /get and /status routes are filtered.
func (s *testingSuite) TestHttpACLHTTPRouteLevel() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx, "httpbin-route", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue,
	)

	s.T().Log("/status route: IP in allowed subnet 10.0.0.0/8 should pass")
	common.BaseGateway.Send(
		s.T(),
		expectAllowed,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "10.5.5.5"),
	)

	s.T().Log("/get route: same allowed IP should also pass")
	common.BaseGateway.Send(
		s.T(),
		expectAllowed,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
		curl.WithHeader("X-Forwarded-For", "10.5.5.5"),
	)

	s.T().Log("/status route: IP outside allowed subnet should be denied by defaultAction=deny")
	common.BaseGateway.Send(
		s.T(),
		expectDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "8.8.8.8"),
	)

	s.T().Log("/get route: same denied IP should also be denied (policy covers all rules)")
	common.BaseGateway.Send(
		s.T(),
		expectDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
		curl.WithHeader("X-Forwarded-For", "8.8.8.8"),
	)
}

// getGatewayPods returns the pod names for the gateway proxy deployment.
func (s *testingSuite) getGatewayPods() []string {
	label := fmt.Sprintf("%s=%s", defaults.WellKnownAppLabel, proxyObjectMeta.GetName())
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(
		s.Ctx,
		proxyObjectMeta.GetNamespace(),
		metav1.ListOptions{LabelSelector: label},
	)
	pods, err := s.TestInstallation.Actions.Kubectl().GetPodsInNsWithLabel(
		s.Ctx,
		proxyObjectMeta.GetNamespace(),
		label,
	)
	s.Require().NoError(err)
	return pods
}

// TestHttpACLDynamicMetadata verifies that the HTTP ACL filter emits dynamic metadata
// under namespace dev.kgateway.http.acl, key blocked-by, visible in access logs.
// Rules: deny 10.0.0.0/8 (named "block-internal-range"), deny 192.168.0.0/16 (unnamed);
// defaultAction=deny; allow 203.0.113.0/24.
func (s *testingSuite) TestHttpACLDynamicMetadata() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx, "httpbin-route", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue,
	)
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPListenerPolicyCondition(
		s.Ctx, "acl-access-log", "kgateway-base", gwv1.GatewayConditionAccepted, metav1.ConditionTrue,
	)

	pods := s.getGatewayPods()

	// Confirm the access log format string has propagated to Envoy before sending requests.
	s.TestInstallation.AssertionsT(s.T()).AssertEnvoyAdminApi(
		s.Ctx,
		proxyObjectMeta,
		func(ctx context.Context, adminClient *admincli.Client) {
			s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
				cfgDump, err := adminClient.GetConfigDump(ctx, nil)
				g.Expect(err).NotTo(gomega.HaveOccurred(), "can get config dump")

				cfgJSON, err := protojson.Marshal(cfgDump)
				g.Expect(err).NotTo(gomega.HaveOccurred(), "can marshal config dump")

				g.Expect(string(cfgJSON)).To(gomega.ContainSubstring("dev.kgateway.http.acl:blocked-by"),
					"access log format string should be present in Envoy config")
			}).WithTimeout(30 * time.Second).WithPolling(time.Second).
				Should(gomega.Succeed())
		},
	)

	s.T().Log("named rule deny: blocked-by should be 'block-internal-range'")
	common.BaseGateway.Send(
		s.T(),
		expectDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "10.5.5.5"),
	)

	s.T().Log("unnamed rule deny: blocked-by should be 'rule'")
	common.BaseGateway.Send(
		s.T(),
		expectDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "192.168.1.1"),
	)

	s.T().Log("default action deny: blocked-by should be 'default'")
	common.BaseGateway.Send(
		s.T(),
		expectDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "8.8.8.8"),
	)

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.TestInstallation.Actions.Kubectl().GetContainerLogs(
			s.Ctx, proxyObjectMeta.GetNamespace(), pods[0],
		)
		s.Require().NoError(err)
		assert.Contains(c, logs, `"blocked_by":"block-internal-range"`)
		assert.Contains(c, logs, `"blocked_by":"rule"`)
		assert.Contains(c, logs, `"blocked_by":"default"`)
	}, 5*time.Second, 100*time.Millisecond)
}

// TestHttpACLLargeRuleset verifies the control plane can accept and apply a TrafficPolicy
// with 200 rules and 20 CIDRs per rule (4000 total), covering IPv4 (/8–/32, 0.0.0.0/0),
// IPv6 short form (::1/128, fe80::/10, ::/0), IPv6 long form (no compression), partial
// compression (2001:db8::x:y), IPv4-mapped (::ffff:...), ULA (fd::/8), multicast (ff::/8),
// and various prefix lengths (/7, /10, /12, /16, /24, /32, /48, /64, /96, /128).
// defaultAction=deny; even-numbered rules (0, 2, 4, ...) are allow, odd rules are deny.
// Rule rule-00000 allows 10.0.0.0/24, so 10.0.0.1 should pass; 8.8.8.8 has no match and is denied.
func (s *testingSuite) TestHttpACLLargeRuleset() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx, "httpbin-route", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue,
	)

	s.T().Log("10.0.0.1 matches rule-00000 (allow, 10.0.0.0/24) → allowed")
	common.BaseGateway.Send(
		s.T(),
		expectAllowed,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "10.0.0.1"),
	)

	s.T().Log("8.8.8.8 matches no rule → denied by defaultAction=deny")
	common.BaseGateway.Send(
		s.T(),
		expectDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "8.8.8.8"),
	)
}

// TestHttpACLValidWithInvalidCIDRPolicy applies a valid ACL policy, verifies it enforces
// allow/deny correctly, then adds a second TrafficPolicy whose CIDR is invalid by having the
// host bits set (172.18.0.0/12) and repeats the same requests to observe how the control plane
// handles the conflict.
func (s *testingSuite) TestHttpACLValidWithInvalidCIDRPolicy() {
	// ── Phase 1: valid ACL policy only ───────────────────────────────────────

	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx, "httpbin-route", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue,
	)

	s.T().Log("Phase 1 — valid ACL only: 10.0.0.1 should be allowed")
	common.BaseGateway.Send(
		s.T(),
		expectAllowed,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "10.0.0.1"),
	)

	s.T().Log("Phase 1 — valid ACL only: 192.168.1.100 should be denied (matches 192.168.0.0/16)")
	common.BaseGateway.Send(
		s.T(),
		expectDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "192.168.1.100"),
	)

	// ── Phase 2: add the invalid ACL policy ──────────────────────────────────

	s.T().Log("Phase 2 — applying invalid-cidr-acl-policy (172.18.0.0/12 has host bits set)")
	s.Require().NoError(
		s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, invalidACLPolicyManifest),
		"apply invalid-acl-policy.yaml",
	)
	defer func() {
		_ = s.TestInstallation.Actions.Kubectl().RunCommand(
			s.Ctx, "delete", "trafficpolicy", "invalid-cidr-acl-policy",
			"-n", "kgateway-base", "--ignore-not-found",
		)
	}()

	// The invalid policy must be rejected with a clear CIDR error in its status.
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		tp := &kgateway.TrafficPolicy{}
		err := s.TestInstallation.ClusterContext.Client.Get(
			s.Ctx,
			types.NamespacedName{Name: "invalid-cidr-acl-policy", Namespace: "kgateway-base"},
			tp,
		)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "can get invalid-cidr-acl-policy TrafficPolicy")
		g.Expect(tp.Status.Ancestors).NotTo(gomega.BeEmpty(), "TrafficPolicy should report ancestor status")

		cond := apimeta.FindStatusCondition(tp.Status.Ancestors[0].Conditions, string(shared.PolicyConditionAccepted))
		g.Expect(cond).NotTo(gomega.BeNil(), "TrafficPolicy should have an Accepted condition")
		g.Expect(cond.Status).To(gomega.Equal(metav1.ConditionFalse), "invalid policy should be Accepted=False")
		g.Expect(cond.Reason).To(gomega.Equal(string(shared.PolicyReasonInvalid)), "reason should be Invalid")
		g.Expect(cond.Message).To(gomega.ContainSubstring("172.18.0.0/12"), "error should name the bad CIDR")
	}).WithTimeout(30 * time.Second).WithPolling(time.Second).Should(gomega.Succeed())

	s.T().Log("Phase 2 — after invalid policy: 10.0.0.1 route being replaced with 500 response")
	common.BaseGateway.Send(
		s.T(),
		expectInternalServerError,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "10.0.0.1"),
	)

	s.T().Log("Phase 2 — after invalid policy: 192.168.1.100 route being replaced with 500 response")
	common.BaseGateway.Send(
		s.T(),
		expectInternalServerError,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "192.168.1.100"),
	)
}

// TestHttpACLGatewayLevel verifies that an ACL policy attached via targetRef to a Gateway
// applies globally across all routes, with named rules and a custom denyResponse.
// defaultAction=allow, block 10.0.0.0/8 (named), allow 10.1.0.0/16 (named).
func (s *testingSuite) TestHttpACLGatewayLevel() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx, "httpbin-route", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue,
	)

	s.T().Log("10.2.0.1 matches block-internal-range (/8) → denied with X-Blocked-By: block-internal-range")
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusForbidden,
			Headers:    map[string]any{"X-Blocked-By": "block-internal-range"},
		},
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "10.2.0.1"),
	)

	s.T().Log("10.1.0.5 matches allow-trusted-subnet (/16, beats /8) → allowed")
	common.BaseGateway.Send(
		s.T(),
		expectAllowed,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "10.1.0.5"),
	)

	s.T().Log("8.8.8.8 matches no rule → allowed by defaultAction=allow")
	common.BaseGateway.Send(
		s.T(),
		expectAllowed,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("X-Forwarded-For", "8.8.8.8"),
	)
}
