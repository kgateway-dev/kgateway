package backendconfigpolicy

import (
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestTranslateHealthCheck(t *testing.T) {
	tests := []struct {
		name     string
		config   *v1alpha1.HealthCheck
		expected *corev3.HealthCheck
	}{
		{
			name:     "nil health check",
			config:   nil,
			expected: nil,
		},
		{
			name: "basic health check config",
			config: &v1alpha1.HealthCheck{
				Timeout:            &metav1.Duration{Duration: 5 * time.Second},
				Interval:           &metav1.Duration{Duration: 10 * time.Second},
				UnhealthyThreshold: ptr.To(uint32(3)),
				HealthyThreshold:   ptr.To(uint32(2)),
			},
			expected: &corev3.HealthCheck{
				Timeout:            durationpb.New(5 * time.Second),
				Interval:           durationpb.New(10 * time.Second),
				UnhealthyThreshold: &wrapperspb.UInt32Value{Value: 3},
				HealthyThreshold:   &wrapperspb.UInt32Value{Value: 2},
			},
		},
		{
			name: "HTTP health check",
			config: &v1alpha1.HealthCheck{
				Timeout:  &metav1.Duration{Duration: 5 * time.Second},
				Interval: &metav1.Duration{Duration: 10 * time.Second},
				Http: &v1alpha1.HealthCheckHttp{
					Host:   ptr.To("example.com"),
					Path:   "/health",
					Method: ptr.To("GET"),
					ExpectedStatuses: []v1alpha1.Int64Range{
						{Start: 200, End: 299},
					},
				},
			},
			expected: &corev3.HealthCheck{
				Timeout:  durationpb.New(5 * time.Second),
				Interval: durationpb.New(10 * time.Second),
				HealthChecker: &corev3.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &corev3.HealthCheck_HttpHealthCheck{
						Host:   "example.com",
						Path:   "/health",
						Method: corev3.RequestMethod_GET,
						ExpectedStatuses: []*typev3.Int64Range{
							{
								Start: 200,
								End:   299,
							},
						},
					},
				},
			},
		},
		{
			name: "gRPC health check",
			config: &v1alpha1.HealthCheck{
				Timeout:  &metav1.Duration{Duration: 5 * time.Second},
				Interval: &metav1.Duration{Duration: 10 * time.Second},
				Grpc: &v1alpha1.HealthCheckGrpc{
					ServiceName: ptr.To("grpc.health.v1.Health"),
					Authority:   ptr.To("example.com"),
				},
			},
			expected: &corev3.HealthCheck{
				Timeout:  durationpb.New(5 * time.Second),
				Interval: durationpb.New(10 * time.Second),
				HealthChecker: &corev3.HealthCheck_GrpcHealthCheck_{
					GrpcHealthCheck: &corev3.HealthCheck_GrpcHealthCheck{
						ServiceName: "grpc.health.v1.Health",
						Authority:   "example.com",
					},
				},
			},
		},
		{
			name: "HTTP health check with multiple status ranges",
			config: &v1alpha1.HealthCheck{
				Timeout:  &metav1.Duration{Duration: 5 * time.Second},
				Interval: &metav1.Duration{Duration: 10 * time.Second},
				Http: &v1alpha1.HealthCheckHttp{
					Host: ptr.To("example.com"),
					Path: "/health",
					ExpectedStatuses: []v1alpha1.Int64Range{
						{Start: 200, End: 299},
						{Start: 401, End: 403},
					},
				},
			},
			expected: &corev3.HealthCheck{
				Timeout:  durationpb.New(5 * time.Second),
				Interval: durationpb.New(10 * time.Second),
				HealthChecker: &corev3.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &corev3.HealthCheck_HttpHealthCheck{
						Host: "example.com",
						Path: "/health",
						ExpectedStatuses: []*typev3.Int64Range{
							{
								Start: 200,
								End:   299,
							},
							{
								Start: 401,
								End:   403,
							},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := translateHealthCheck(test.config)
			if !proto.Equal(result, test.expected) {
				t.Errorf("expected %v, got %v", test.expected, result)
			}
		})
	}
}
