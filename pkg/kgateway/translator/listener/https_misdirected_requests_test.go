package listener

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestShouldShadowHTTPSVirtualHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		currentPattern string
		hostPattern    string
		allPatterns    []string
		want           bool
	}{
		{
			name:           "catch-all listener shadows exact host owned by exact sibling",
			currentPattern: catchAllHostnamePattern,
			hostPattern:    "second-example.org",
			allPatterns:    []string{catchAllHostnamePattern, "second-example.org"},
			want:           true,
		},
		{
			name:           "catch-all listener shadows exact host owned by wildcard sibling",
			currentPattern: catchAllHostnamePattern,
			hostPattern:    "third-example.wildcard.org",
			allPatterns:    []string{catchAllHostnamePattern, "*.wildcard.org"},
			want:           true,
		},
		{
			name:           "wildcard listener shadows exact host owned by exact sibling",
			currentPattern: "*.wildcard.org",
			hostPattern:    "fourth-example.wildcard.org",
			allPatterns:    []string{"*.wildcard.org", "fourth-example.wildcard.org"},
			want:           true,
		},
		{
			name:           "wildcard listener keeps exact host inside its own hostspace",
			currentPattern: "*.wildcard.org",
			hostPattern:    "third-example.wildcard.org",
			allPatterns:    []string{"*.wildcard.org", "fourth-example.wildcard.org"},
			want:           false,
		},
		{
			name:           "exact listener keeps its own host over catch-all sibling",
			currentPattern: "second-example.org",
			hostPattern:    "second-example.org",
			allPatterns:    []string{"second-example.org", catchAllHostnamePattern},
			want:           false,
		},
		{
			name:           "catch-all listener shadows wildcard hostspace owned by wildcard sibling",
			currentPattern: catchAllHostnamePattern,
			hostPattern:    "*.example.org",
			allPatterns:    []string{catchAllHostnamePattern, "*.example.org"},
			want:           true,
		},
		{
			name:           "catch-all listener keeps wildcard hostspace when sibling exact only covers a subset",
			currentPattern: catchAllHostnamePattern,
			hostPattern:    "*.example.org",
			allPatterns:    []string{catchAllHostnamePattern, "foo.example.org"},
			want:           false,
		},
		{
			name:           "wildcard listener keeps broader wildcard when sibling only covers a narrower subset",
			currentPattern: "*.example.org",
			hostPattern:    "*.example.org",
			allPatterns:    []string{"*.example.org", "*.bar.example.org"},
			want:           false,
		},
		{
			name:           "catch-all wildcard route remains when no sibling owns all hostspace",
			currentPattern: catchAllHostnamePattern,
			hostPattern:    catchAllHostnamePattern,
			allPatterns:    []string{catchAllHostnamePattern, "*.example.org"},
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, shouldShadowHTTPSVirtualHost(tt.currentPattern, tt.hostPattern, tt.allPatterns))
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

func TestNeedsProtective404VirtualHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		currentPattern string
		siblings       []string
		want           bool
	}{
		{
			name:           "exact listener needs protection from catch-all sibling",
			currentPattern: "second-example.org",
			siblings:       []string{catchAllHostnamePattern},
			want:           true,
		},
		{
			name:           "exact listener needs protection from broader wildcard sibling",
			currentPattern: "foo.example.org",
			siblings:       []string{"*.example.org"},
			want:           true,
		},
		{
			name:           "wildcard listener needs protection from catch-all sibling",
			currentPattern: "*.wildcard.org",
			siblings:       []string{catchAllHostnamePattern},
			want:           true,
		},
		{
			name:           "wildcard listener does not protect against more specific exact sibling",
			currentPattern: "*.wildcard.org",
			siblings:       []string{"fourth-example.wildcard.org"},
			want:           false,
		},
		{
			name:           "catch-all listener does not need a protective vhost",
			currentPattern: catchAllHostnamePattern,
			siblings:       []string{"second-example.org"},
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, needsProtective404VirtualHost(tt.currentPattern, tt.siblings))
		})
	}
}

func TestBuildHTTPSMisdirectedRequestVirtualHosts(t *testing.T) {
	t.Parallel()

	t.Run("adds sibling 421 responses and current 404 protection when needed", func(t *testing.T) {
		t.Parallel()

		virtualHosts := buildHTTPSMisdirectedRequestVirtualHosts(
			"https-with-hostname",
			ir.Listener{},
			"second-example.org",
			[]string{catchAllHostnamePattern, "*.example.org"},
			map[string]struct{}{},
		)

		require.Len(t, virtualHosts, 3)
		assert.Equal(t, catchAllHostnamePattern, virtualHosts[0].Hostname)
		assert.Equal(t, uint32(http.StatusMisdirectedRequest), virtualHosts[0].DirectResponse.StatusCode)
		assert.Equal(t, "*.example.org", virtualHosts[1].Hostname)
		assert.Equal(t, uint32(http.StatusMisdirectedRequest), virtualHosts[1].DirectResponse.StatusCode)
		assert.Equal(t, "second-example.org", virtualHosts[2].Hostname)
		assert.Equal(t, uint32(http.StatusNotFound), virtualHosts[2].DirectResponse.StatusCode)
	})

	t.Run("skips the protective 404 when the current hostspace already has a vhost", func(t *testing.T) {
		t.Parallel()

		virtualHosts := buildHTTPSMisdirectedRequestVirtualHosts(
			"https-with-wildcard-hostname",
			ir.Listener{},
			"*.wildcard.org",
			[]string{catchAllHostnamePattern},
			map[string]struct{}{"*.wildcard.org": {}},
		)

		require.Len(t, virtualHosts, 1)
		assert.Equal(t, catchAllHostnamePattern, virtualHosts[0].Hostname)
		assert.Equal(t, uint32(http.StatusMisdirectedRequest), virtualHosts[0].DirectResponse.StatusCode)
	})

	t.Run("skips synthetic sibling vhosts that would duplicate actual domains", func(t *testing.T) {
		t.Parallel()

		virtualHosts := buildHTTPSMisdirectedRequestVirtualHosts(
			"https",
			ir.Listener{},
			catchAllHostnamePattern,
			[]string{"*.example.org"},
			map[string]struct{}{"*.example.org": {}},
		)

		require.Empty(t, virtualHosts)
	})
}
