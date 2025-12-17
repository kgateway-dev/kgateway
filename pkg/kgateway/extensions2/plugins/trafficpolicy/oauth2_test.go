package trafficpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedirectPath(t *testing.T) {
	tests := []struct {
		uri     string
		want    string
		wantErr string
	}{
		{
			uri:  defaultRedictURI,
			want: "/oauth2/redirect",
		},
		{
			uri:  "https://foo.com/bar/baz",
			want: "/bar/baz",
		},
		{
			uri:     "foo.com/bar/baz",
			want:    "",
			wantErr: "missing scheme",
		},
		{
			uri:     "https://foo.com/",
			want:    "",
			wantErr: "missing path",
		},
	}
	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			a := assert.New(t)
			path, err := parseRedirectPath(tt.uri)
			a.Equal(tt.want, path)
			if tt.wantErr != "" {
				a.ErrorContains(err, tt.wantErr)
			} else {
				a.NoError(err)
			}
		})
	}
}
