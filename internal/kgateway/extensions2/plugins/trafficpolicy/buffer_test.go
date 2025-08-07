package trafficpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func TestBufferIREquals(t *testing.T) {
	tests := []struct {
		name string
		a, b *v1alpha1.Buffer
		want bool
	}{
		{
			name: "both nil are equal",
			want: true,
		},
		{
			name: "non-nil and not equal",
			a: &v1alpha1.Buffer{
				MaxRequestSize: ptr.To(resource.MustParse("1Ki")),
			},
			b: &v1alpha1.Buffer{
				MaxRequestSize: ptr.To(resource.MustParse("2Ki")),
			},
			want: false,
		},
		{
			name: "non-nil and equal",
			a: &v1alpha1.Buffer{
				MaxRequestSize: ptr.To(resource.MustParse("1Ki")),
			},
			b: &v1alpha1.Buffer{
				MaxRequestSize: ptr.To(resource.MustParse("1Ki")),
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)

			aOut := &trafficPolicySpecIr{}
			constructBuffer(v1alpha1.TrafficPolicySpec{
				Buffer: tt.a,
			}, aOut)

			bOut := &trafficPolicySpecIr{}
			constructBuffer(v1alpha1.TrafficPolicySpec{
				Buffer: tt.b,
			}, bOut)

			a.Equal(tt.want, aOut.buffer.Equals(bOut.buffer))
		})
	}
}
