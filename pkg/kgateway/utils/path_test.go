package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestNormalizePathMatch(t *testing.T) {
	tests := []struct {
		name     string
		pathType gwv1.PathMatchType
		path     string
		want     string
	}{
		{
			name:     "prefix drops trailing slash",
			pathType: gwv1.PathMatchPathPrefix,
			path:     "/wut/",
			want:     "/wut",
		},
		{
			name:     "prefix keeps root",
			pathType: gwv1.PathMatchPathPrefix,
			path:     "/",
			want:     "/",
		},
		{
			name:     "exact keeps trailing slash",
			pathType: gwv1.PathMatchExact,
			path:     "/wut/",
			want:     "/wut/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizePathMatch(tt.pathType, tt.path))
		})
	}
}
