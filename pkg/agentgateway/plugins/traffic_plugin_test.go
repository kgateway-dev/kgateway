package plugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToJSONValue(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{
			name: "regular string",
			in:   `hello`,
			want: `"hello"`,
		},
		{
			name: "JSON string",
			in:   `"hello"`,
			want: `"hello"`,
		},
		{
			name: "JSON number",
			in:   `0.8`,
			want: `0.8`,
		},
		{
			name:    "invalid JSON value",
			in:      `{"key": "value"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)

			got, err := toJSONValue(tt.in)
			if tt.wantErr {
				a.Error(err)
				return
			}
			a.NoError(err)
			a.JSONEq(tt.want, got)
		})
	}
}
