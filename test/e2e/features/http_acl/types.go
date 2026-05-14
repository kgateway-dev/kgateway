//go:build e2e

package http_acl

import (
	"net/http"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var (
	// manifests
	setupManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	defaultAllowManifest     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "default-allow.yaml")
	defaultDenyManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "default-deny.yaml")
	holePunchManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "hole-punch.yaml")
	customDenyRespManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "custom-deny-response.yaml")
	blockedByHeaderManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "blocked-by-header.yaml")
	routeHTTPACLManifest     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route-http-acl.yaml")
	httprouteHTTPACLManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproute-http-acl.yaml")
	gatewayHTTPACLManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway-http-acl.yaml")
	aclAccessLogManifest     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "acl-access-log.yaml")
	largeRulesetManifest     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "large-ruleset.yaml")
	validACLOnlyManifest     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "valid-acl-only.yaml")
	invalidACLPolicyManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "invalid-acl-policy.yaml")

	// proxyObjectMeta targets the shared gateway deployment for Envoy admin API access.
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gateway",
		Namespace: "kgateway-base",
	}

	// expected responses
	expectAllowed = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
	}
	expectDenied = &testmatchers.HttpResponse{
		StatusCode: http.StatusForbidden,
	}
	expectInternalServerError = &testmatchers.HttpResponse{
		StatusCode: http.StatusInternalServerError,
	}

	// setup is the common infrastructure shared across all test cases.
	setup = base.TestCase{
		Manifests: []string{setupManifest},
	}

	testCases = map[string]*base.TestCase{
		"TestHttpACLDefaultAllowDenyCIDR": {
			Manifests: []string{defaultAllowManifest},
		},
		"TestHttpACLDefaultDenyAllowSubnet": {
			Manifests: []string{defaultDenyManifest},
		},
		"TestHttpACLHolePunchNamedRules": {
			Manifests: []string{holePunchManifest},
		},
		"TestHttpACLCustomDenyResponse": {
			Manifests: []string{customDenyRespManifest},
		},
		"TestHttpACLBlockedByHeader": {
			Manifests: []string{blockedByHeaderManifest},
		},
		"TestHttpACLBlockedCounter": {
			Manifests: []string{defaultDenyManifest},
		},
		"TestHttpACLRouteLevel": {
			Manifests: []string{routeHTTPACLManifest},
		},
		"TestHttpACLHTTPRouteLevel": {
			Manifests: []string{httprouteHTTPACLManifest},
		},
		"TestHttpACLGatewayLevel": {
			Manifests: []string{gatewayHTTPACLManifest},
		},
		"TestHttpACLDynamicMetadata": {
			Manifests: []string{aclAccessLogManifest},
		},
		"TestHttpACLLargeRuleset": {
			Manifests: []string{largeRulesetManifest},
		},
		// Only the valid-policy manifest is registered here so the framework
		// auto-cleans up the HTTPRoute and valid TrafficPolicy after the test.
		// The invalid TrafficPolicy is applied mid-test and cleaned up via defer.
		"TestHttpACLValidWithInvalidCIDRPolicy": {
			Manifests: []string{validACLOnlyManifest},
		},
	}
)
