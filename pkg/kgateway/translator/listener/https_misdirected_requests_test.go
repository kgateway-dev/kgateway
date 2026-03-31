package listener

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestHostnamePatternMatchesHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		host    string
		want    bool
	}{
		{
			name:    "wildcard matches single label",
			pattern: "*.example.org",
			host:    "a.example.org",
			want:    true,
		},
		{
			name:    "wildcard matches multiple labels",
			pattern: "*.example.org",
			host:    "a.b.example.org",
			want:    true,
		},
		{
			name:    "wildcard does not match bare suffix",
			pattern: "*.example.org",
			host:    "example.org",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, hostnamePatternMatchesHost(tt.pattern, tt.host))
		})
	}
}

func TestHostnamePatternContains(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		containerPattern string
		hostPattern      string
		want             bool
	}{
		{
			name:             "catch-all contains wildcard",
			containerPattern: catchAllHostnamePattern,
			hostPattern:      "*.example.org",
			want:             true,
		},
		{
			name:             "wildcard contains narrower wildcard",
			containerPattern: "*.example.org",
			hostPattern:      "*.bar.example.org",
			want:             true,
		},
		{
			name:             "wildcard does not contain broader wildcard",
			containerPattern: "*.bar.example.org",
			hostPattern:      "*.example.org",
			want:             false,
		},
		{
			name:             "exact does not contain wildcard",
			containerPattern: "foo.example.org",
			hostPattern:      "*.example.org",
			want:             false,
		},
		{
			name:             "wildcard contains matching exact host",
			containerPattern: "*.example.org",
			hostPattern:      "foo.example.org",
			want:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, hostnamePatternContains(tt.containerPattern, tt.hostPattern))
		})
	}
}

func TestAuthorityRegexForHostnamePattern(t *testing.T) {
	t.Parallel()

	t.Run("exact hostname matches optional port", func(t *testing.T) {
		t.Parallel()

		matcher := regexp.MustCompile(authorityRegexForHostnamePattern("second-example.org"))
		assert.True(t, matcher.MatchString("second-example.org"))
		assert.True(t, matcher.MatchString("second-example.org:443"))
		assert.False(t, matcher.MatchString("other-example.org"))
	})

	t.Run("wildcard hostname follows suffix semantics", func(t *testing.T) {
		t.Parallel()

		matcher := regexp.MustCompile(authorityRegexForHostnamePattern("*.example.org"))
		assert.True(t, matcher.MatchString("a.example.org"))
		assert.True(t, matcher.MatchString("a.b.example.org"))
		assert.True(t, matcher.MatchString("a.b.example.org:8443"))
		assert.False(t, matcher.MatchString("example.org"))
		assert.False(t, matcher.MatchString("other.org"))
	})
}

func TestBuildHTTPSMisdirectedRequestPlan(t *testing.T) {
	t.Parallel()

	t.Run("catch-all listener uses residual routes for uncovered overlap only", func(t *testing.T) {
		t.Parallel()

		plan := buildHTTPSMisdirectedRequestPlan(
			catchAllHostnamePattern,
			[]string{"second-example.org", "*.wildcard.org", "fourth-example.wildcard.org"},
			[]string{"example.org"},
		)

		require.Empty(t, plan.routesByDomain["example.org"])
		require.Len(t, plan.residualRoutes, 3)
		assertDirectResponseRoute(t, plan.residualRoutes[0], http.StatusMisdirectedRequest, authorityRegexForHostnamePattern("second-example.org"))
		assertDirectResponseRoute(t, plan.residualRoutes[1], http.StatusMisdirectedRequest, authorityRegexForHostnamePattern("*.wildcard.org"))
		assertDirectResponseRoute(t, plan.residualRoutes[2], http.StatusNotFound, "")
	})

	t.Run("catch-all actual vhost gets the minimized overlap route set", func(t *testing.T) {
		t.Parallel()

		plan := buildHTTPSMisdirectedRequestPlan(
			catchAllHostnamePattern,
			[]string{"*.example.org", "foo.example.org"},
			[]string{catchAllHostnamePattern},
		)

		require.Len(t, plan.routesByDomain, 1)
		require.Len(t, plan.routesByDomain[catchAllHostnamePattern], 1)
		assertDirectResponseRoute(t, plan.routesByDomain[catchAllHostnamePattern][0], http.StatusMisdirectedRequest, authorityRegexForHostnamePattern("*.example.org"))
		require.Empty(t, plan.residualRoutes)
	})

	t.Run("wildcard listener injects exact 421 into the wildcard vhost", func(t *testing.T) {
		t.Parallel()

		plan := buildHTTPSMisdirectedRequestPlan(
			"*.wildcard.org",
			[]string{"fourth-example.wildcard.org"},
			[]string{"*.wildcard.org"},
		)

		require.Len(t, plan.routesByDomain["*.wildcard.org"], 1)
		assertDirectResponseRoute(t, plan.routesByDomain["*.wildcard.org"][0], http.StatusMisdirectedRequest, authorityRegexForHostnamePattern("fourth-example.wildcard.org"))
		require.Empty(t, plan.residualRoutes)
	})

	t.Run("exact listener with catch-all sibling gets protective 404 and fallback 421", func(t *testing.T) {
		t.Parallel()

		plan := buildHTTPSMisdirectedRequestPlan(
			"second-example.org",
			[]string{catchAllHostnamePattern},
			nil,
		)

		require.Len(t, plan.residualRoutes, 2)
		assertDirectResponseRoute(t, plan.residualRoutes[0], http.StatusNotFound, authorityRegexForHostnamePattern("second-example.org"))
		assertDirectResponseRoute(t, plan.residualRoutes[1], http.StatusMisdirectedRequest, "")
	})

	t.Run("exact listener with wildcard sibling protects its own hostname before 421 overlap routes", func(t *testing.T) {
		t.Parallel()

		plan := buildHTTPSMisdirectedRequestPlan(
			"foo.example.org",
			[]string{"*.example.org"},
			nil,
		)

		require.Len(t, plan.residualRoutes, 3)
		assertDirectResponseRoute(t, plan.residualRoutes[0], http.StatusNotFound, authorityRegexForHostnamePattern("foo.example.org"))
		assertDirectResponseRoute(t, plan.residualRoutes[1], http.StatusMisdirectedRequest, authorityRegexForHostnamePattern("*.example.org"))
		assertDirectResponseRoute(t, plan.residualRoutes[2], http.StatusNotFound, "")
	})
}

func TestApplyHTTPSMisdirectedRequestRoutes(t *testing.T) {
	t.Parallel()

	virtualHosts := []*ir.VirtualHost{
		{
			Name:     "https~foo_example_org",
			Hostname: "foo.example.org",
			Rules: []ir.HttpRouteRuleMatchIR{
				{Name: "real-route"},
			},
		},
	}

	out := applyHTTPSMisdirectedRequestRoutes(
		"https",
		ir.Listener{},
		catchAllHostnamePattern,
		[]string{"*.example.org"},
		virtualHosts,
	)

	require.Len(t, out, 2)
	fooVhost := out[0]
	require.Len(t, fooVhost.Rules, 2)
	assertDirectResponseRoute(t, fooVhost.Rules[0], http.StatusMisdirectedRequest, authorityRegexForHostnamePattern("foo.example.org"))
	assert.Equal(t, "real-route", fooVhost.Rules[1].Name)

	residualVhost := out[1]
	assert.Equal(t, catchAllHostnamePattern, residualVhost.Hostname)
	require.Len(t, residualVhost.Rules, 2)
	assertDirectResponseRoute(t, residualVhost.Rules[0], http.StatusMisdirectedRequest, authorityRegexForHostnamePattern("*.example.org"))
	assertDirectResponseRoute(t, residualVhost.Rules[1], http.StatusNotFound, "")
}

func assertDirectResponseRoute(t *testing.T, route ir.HttpRouteRuleMatchIR, wantStatus uint32, wantAuthorityRegex string) {
	t.Helper()

	require.NotNil(t, route.DirectResponse)
	assert.Equal(t, wantStatus, route.DirectResponse.StatusCode)

	if wantAuthorityRegex == "" {
		assert.Empty(t, route.Match.Headers)
		return
	}

	require.Len(t, route.Match.Headers, 1)
	assert.Equal(t, gwAuthorityHeaderName, string(route.Match.Headers[0].Name))
	assert.Equal(t, wantAuthorityRegex, route.Match.Headers[0].Value)
}

const gwAuthorityHeaderName = ":authority"
