package stringutils_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/stringutils"
)

func TestIsRestrictedHeaderName(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		restricted bool
	}{
		{name: "ordinary header", header: "X-Custom-Header", restricted: false},
		{name: "host exact case", header: "Host", restricted: true},
		{name: "host lowercase", header: "host", restricted: true},
		{name: "host uppercase", header: "HOST", restricted: true},
		{name: "host mixed case", header: "HoSt", restricted: true},
		{name: "authority pseudo-header", header: ":authority", restricted: true},
		{name: "path pseudo-header", header: ":path", restricted: true},
		{name: "method pseudo-header", header: ":method", restricted: true},
		{name: "scheme pseudo-header", header: ":scheme", restricted: true},
		{name: "status pseudo-header", header: ":status", restricted: true},
		{name: "header containing but not starting with colon", header: "X-Foo:Bar", restricted: false},
		{name: "header containing host as substring", header: "X-Host-Region", restricted: false},
		{name: "empty string", header: "", restricted: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.restricted, stringutils.IsRestrictedHeaderName(tt.header))
		})
	}
}
