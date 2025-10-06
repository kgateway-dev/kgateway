package backendconfigpolicy

import (
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestTranslateOutlierDetection(t *testing.T) {
	tests := []struct {
		name     string
		config   *v1alpha1.OutlierDetection
		expected *envoyclusterv3.OutlierDetection
	}{
		{
			name:     "nil outlier detection",
			config:   nil,
			expected: nil,
		},
		{
			name:     "minimalist outlier detection config",
			config:   &v1alpha1.OutlierDetection{},
			expected: &envoyclusterv3.OutlierDetection{},
		},
		{
			name: "partial outlier detection config",
			config: &v1alpha1.OutlierDetection{
				Interval: &metav1.Duration{Duration: 11 * time.Second},
			},
			expected: &envoyclusterv3.OutlierDetection{
				Interval: durationpb.New(11 * time.Second),
			},
		},
		{
			name: "full outlier detection config",
			config: &v1alpha1.OutlierDetection{
				Consecutive5xx:     ptr.To(int32(2)),
				Interval:           &metav1.Duration{Duration: 5 * time.Second},
				BaseEjectionTime:   &metav1.Duration{Duration: 7 * time.Minute},
				MaxEjectionPercent: ptr.To(int32(99)),
			},
			expected: &envoyclusterv3.OutlierDetection{
				Consecutive_5Xx:    &wrapperspb.UInt32Value{Value: 2},
				Interval:           durationpb.New(5 * time.Second),
				BaseEjectionTime:   durationpb.New(7 * time.Minute),
				MaxEjectionPercent: &wrapperspb.UInt32Value{Value: 99},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := translateOutlierDetection(test.config)
			if !proto.Equal(result, test.expected) {
				t.Errorf("expected %v, got %v", test.expected, result)
			}
		})
	}
}
