package deployer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComponentLogLevelsToString(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]string
		want    string
		wantErr error
	}{
		{
			name:    "empty map should convert to empty string",
			input:   map[string]string{},
			want:    "",
			wantErr: nil,
		},
		{
			name:    "empty key should throw error",
			input:   map[string]string{"": "val"},
			want:    "",
			wantErr: ComponentLogLevelEmptyError("", "val"),
		},
		{
			name:    "empty value should throw error",
			input:   map[string]string{"key": ""},
			want:    "",
			wantErr: ComponentLogLevelEmptyError("key", ""),
		},
		{
			name: "should sort keys",
			input: map[string]string{
				"bbb": "val1",
				"cat": "val2",
				"a":   "val3",
			},
			want:    "a:val3,bbb:val1,cat:val2",
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ComponentLogLevelsToString(tt.input)
			if tt.wantErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.wantErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
