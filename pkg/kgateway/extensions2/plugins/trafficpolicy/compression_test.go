package trafficpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func TestCompressionIREquals(t *testing.T) {
	tests := []struct {
		name string
		a, b *compressionIR
		want bool
	}{
		{
			name: "both nil are equal",
			want: true,
		},
		{
			name: "nil and non-nil are not equal",
			a:    &compressionIR{enable: true, libraries: []kgateway.CompressionLibrary{kgateway.CompressionGzip}},
			want: false,
		},
		{
			name: "same enable and libraries are equal",
			a:    &compressionIR{enable: true, libraries: []kgateway.CompressionLibrary{kgateway.CompressionBrotli, kgateway.CompressionGzip}},
			b:    &compressionIR{enable: true, libraries: []kgateway.CompressionLibrary{kgateway.CompressionBrotli, kgateway.CompressionGzip}},
			want: true,
		},
		{
			name: "different library contents are not equal",
			a:    &compressionIR{enable: true, libraries: []kgateway.CompressionLibrary{kgateway.CompressionGzip}},
			b:    &compressionIR{enable: true, libraries: []kgateway.CompressionLibrary{kgateway.CompressionZstd}},
			want: false,
		},
		{
			name: "different library order is not equal",
			a:    &compressionIR{enable: true, libraries: []kgateway.CompressionLibrary{kgateway.CompressionBrotli, kgateway.CompressionGzip}},
			b:    &compressionIR{enable: true, libraries: []kgateway.CompressionLibrary{kgateway.CompressionGzip, kgateway.CompressionBrotli}},
			want: false,
		},
		{
			name: "different library length is not equal",
			a:    &compressionIR{enable: true, libraries: []kgateway.CompressionLibrary{kgateway.CompressionBrotli, kgateway.CompressionGzip}},
			b:    &compressionIR{enable: true, libraries: []kgateway.CompressionLibrary{kgateway.CompressionBrotli}},
			want: false,
		},
		{
			name: "different enable is not equal",
			a:    &compressionIR{enable: true, libraries: []kgateway.CompressionLibrary{kgateway.CompressionGzip}},
			b:    &compressionIR{enable: false, libraries: []kgateway.CompressionLibrary{kgateway.CompressionGzip}},
			want: false,
		},
		{
			name: "disabled ignores libraries",
			a:    &compressionIR{enable: false, libraries: []kgateway.CompressionLibrary{kgateway.CompressionGzip}},
			b:    &compressionIR{enable: false, libraries: []kgateway.CompressionLibrary{kgateway.CompressionBrotli}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.a.Equals(tt.b))
		})
	}
}

func TestConstructCompressionLibraries(t *testing.T) {
	tests := []struct {
		name      string
		libraries []kgateway.CompressionLibrary
		wantLibs  []kgateway.CompressionLibrary
	}{
		{
			name:      "unset libraries default to gzip for backward compatibility",
			libraries: nil,
			wantLibs:  []kgateway.CompressionLibrary{kgateway.CompressionGzip},
		},
		{
			name:      "single explicit codec is preserved",
			libraries: []kgateway.CompressionLibrary{kgateway.CompressionBrotli},
			wantLibs:  []kgateway.CompressionLibrary{kgateway.CompressionBrotli},
		},
		{
			name:      "multiple codecs preserve preference order",
			libraries: []kgateway.CompressionLibrary{kgateway.CompressionBrotli, kgateway.CompressionGzip},
			wantLibs:  []kgateway.CompressionLibrary{kgateway.CompressionBrotli, kgateway.CompressionGzip},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &trafficPolicySpecIr{}
			constructCompression(kgateway.TrafficPolicySpec{
				Compression: &kgateway.Compression{
					ResponseCompression: &kgateway.ResponseCompression{
						Libraries: tt.libraries,
					},
				},
			}, out)

			if assert.NotNil(t, out.compression) {
				assert.True(t, out.compression.enable)
				assert.Equal(t, tt.wantLibs, out.compression.libraries)
			}
		})
	}
}
