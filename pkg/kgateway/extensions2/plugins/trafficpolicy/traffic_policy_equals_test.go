package trafficpolicy

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// subIRSetters sets one trafficPolicySpecIr field to a non-nil, empty sub-IR.
var subIRSetters = map[string]func(*trafficPolicySpecIr){
	"buffer":           func(s *trafficPolicySpecIr) { s.buffer = &bufferIR{} },
	"extProc":          func(s *trafficPolicySpecIr) { s.extProc = &extprocIR{} },
	"rustformation":    func(s *trafficPolicySpecIr) { s.rustformation = &rustformationIR{} },
	"extAuth":          func(s *trafficPolicySpecIr) { s.extAuth = &extAuthIR{} },
	"localRateLimit":   func(s *trafficPolicySpecIr) { s.localRateLimit = &localRateLimitIR{} },
	"globalRateLimit":  func(s *trafficPolicySpecIr) { s.globalRateLimit = &globalRateLimitIR{} },
	"cors":             func(s *trafficPolicySpecIr) { s.cors = &corsIR{} },
	"csrf":             func(s *trafficPolicySpecIr) { s.csrf = &csrfIR{} },
	"headerModifiers":  func(s *trafficPolicySpecIr) { s.headerModifiers = &headerModifiersIR{} },
	"requestMirror":    func(s *trafficPolicySpecIr) { s.requestMirror = &requestMirrorIR{} },
	"autoHostRewrite":  func(s *trafficPolicySpecIr) { s.autoHostRewrite = &autoHostRewriteIR{} },
	"retry":            func(s *trafficPolicySpecIr) { s.retry = &retryIR{} },
	"timeouts":         func(s *trafficPolicySpecIr) { s.timeouts = &timeoutsIR{} },
	"internalRedirect": func(s *trafficPolicySpecIr) { s.internalRedirect = &internalRedirectIR{} },
	"rbac":             func(s *trafficPolicySpecIr) { s.rbac = &rbacIR{} },
	"jwt":              func(s *trafficPolicySpecIr) { s.jwt = &jwtIr{} },
	"compression":      func(s *trafficPolicySpecIr) { s.compression = &compressionIR{} },
	"decompression":    func(s *trafficPolicySpecIr) { s.decompression = &decompressionIR{} },
	"basicAuth":        func(s *trafficPolicySpecIr) { s.basicAuth = &basicAuthIR{} },
	"urlRewrite":       func(s *trafficPolicySpecIr) { s.urlRewrite = &urlRewriteIR{} },
	"apiKeyAuth":       func(s *trafficPolicySpecIr) { s.apiKeyAuth = &apiKeyAuthIR{} },
	"oauth2":           func(s *trafficPolicySpecIr) { s.oauth2 = &oauthIR{} },
	"tracing":          func(s *trafficPolicySpecIr) { s.tracing = &routeTracingIR{} },
	"faultInjection":   func(s *trafficPolicySpecIr) { s.faultInjection = &faultInjectionIR{} },
	"httpACL":          func(s *trafficPolicySpecIr) { s.httpACL = &httpACLIR{} },
	"statPrefix":       func(s *trafficPolicySpecIr) { s.statPrefix = &statPrefixIR{} },
	"httpUpgrade":      func(s *trafficPolicySpecIr) { s.httpUpgrade = &httpUpgradeIR{} },
}

// TestTrafficPolicyEqualsWithOneSubIRUnset compares a policy that sets one
// sub-IR with one that leaves it unset, in both directions. The unset side
// reaches the sub-IR's Equals as a nil pointer inside the PolicySubIR
// interface, which is not a nil interface, so an Equals that checks the
// interface for nil instead of the asserted pointer dereferences nil. KRT
// compares old against new in some collections and new against old in others,
// so both directions run in production.
func TestTrafficPolicyEqualsWithOneSubIRUnset(t *testing.T) {
	spec := reflect.TypeFor[trafficPolicySpecIr]()
	for field := range spec.Fields() {
		require.Contains(t, subIRSetters, field.Name, "add a setter for the new sub-IR field")
	}
	require.Len(t, subIRSetters, spec.NumField())

	for name, set := range subIRSetters {
		t.Run(name, func(t *testing.T) {
			withField, without := &TrafficPolicy{}, &TrafficPolicy{}
			set(&withField.spec)

			assert.NotPanics(t, func() {
				assert.False(t, withField.Equals(without), "set compared with unset")
				assert.False(t, without.Equals(withField), "unset compared with set")
			})
			assert.True(t, without.Equals(&TrafficPolicy{}), "both unset")
		})
	}
}
