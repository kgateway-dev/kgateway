package listenerpolicy

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func TestValidateHTTP2ProtocolOptions(t *testing.T) {
	tests := []struct {
		name        string
		options     *kgateway.ListenerHTTP2ProtocolOptions
		expectedErr []string
	}{
		{
			name: "valid window sizes",
			options: &kgateway.ListenerHTTP2ProtocolOptions{
				InitialStreamWindowSize:     new(resource.MustParse("128Ki")),
				InitialConnectionWindowSize: new(resource.MustParse("256Ki")),
			},
		},
		{
			name: "rejects stream window size below minimum",
			options: &kgateway.ListenerHTTP2ProtocolOptions{
				InitialStreamWindowSize: new(resource.MustParse("65534")),
			},
			expectedErr: []string{
				"initialStreamWindowSize must be between 65535 and 2147483647 bytes (inclusive), got 65534",
			},
		},
		{
			name: "rejects connection window size above maximum",
			options: &kgateway.ListenerHTTP2ProtocolOptions{
				InitialConnectionWindowSize: new(resource.MustParse("2147483648")),
			},
			expectedErr: []string{
				"initialConnectionWindowSize must be between 65535 and 2147483647 bytes (inclusive), got 2147483648",
			},
		},
		{
			name: "reports both invalid fields",
			options: &kgateway.ListenerHTTP2ProtocolOptions{
				InitialStreamWindowSize:     new(resource.MustParse("1")),
				InitialConnectionWindowSize: new(resource.MustParse("2147483648")),
			},
			expectedErr: []string{
				"initialStreamWindowSize must be between 65535 and 2147483647 bytes (inclusive), got 1",
				"initialConnectionWindowSize must be between 65535 and 2147483647 bytes (inclusive), got 2147483648",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateHTTP2ProtocolOptions(tt.options)
			require.Len(t, errs, len(tt.expectedErr))
			for i, err := range errs {
				require.EqualError(t, err, tt.expectedErr[i])
			}
		})
	}
}
