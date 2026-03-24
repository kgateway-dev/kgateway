package listener

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestShouldShadowHTTPSExactVirtualHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		currentPattern string
		host           string
		allPatterns    []string
		want           bool
	}{
		{
			name:           "catch-all listener shadows exact host owned by exact sibling",
			currentPattern: catchAllHostnamePattern,
			host:           "second-example.org",
			allPatterns:    []string{catchAllHostnamePattern, "second-example.org"},
			want:           true,
		},
		{
			name:           "catch-all listener shadows exact host owned by wildcard sibling",
			currentPattern: catchAllHostnamePattern,
			host:           "third-example.wildcard.org",
			allPatterns:    []string{catchAllHostnamePattern, "*.wildcard.org"},
			want:           true,
		},
		{
			name:           "wildcard listener shadows exact host owned by exact sibling",
			currentPattern: "*.wildcard.org",
			host:           "fourth-example.wildcard.org",
			allPatterns:    []string{"*.wildcard.org", "fourth-example.wildcard.org"},
			want:           true,
		},
		{
			name:           "wildcard listener keeps exact host inside its own hostspace",
			currentPattern: "*.wildcard.org",
			host:           "third-example.wildcard.org",
			allPatterns:    []string{"*.wildcard.org", "fourth-example.wildcard.org"},
			want:           false,
		},
		{
			name:           "exact listener keeps its own host over catch-all sibling",
			currentPattern: "second-example.org",
			host:           "second-example.org",
			allPatterns:    []string{"second-example.org", catchAllHostnamePattern},
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, shouldShadowHTTPSExactVirtualHost(tt.currentPattern, tt.host, tt.allPatterns))
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
			context.Background(),
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
			context.Background(),
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
}
