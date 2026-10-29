package backendconfigpolicy

import (
	"testing"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func TestTranslateHealthCheck(t *testing.T) {
	tests := []struct {
		name     string
		config   *kgateway.HealthCheck
		expected *envoycorev3.HealthCheck
	}{
		{
			name:     "nil health check",
			config:   nil,
			expected: nil,
		},
		{
			name: "basic health check config",
			config: &kgateway.HealthCheck{
				Timeout:            metav1.Duration{Duration: 5 * time.Second},
				Interval:           metav1.Duration{Duration: 10 * time.Second},
				UnhealthyThreshold: int32(3),
				HealthyThreshold:   int32(2),
				Http: &kgateway.HealthCheckHttp{
					Path: "/health",
				},
			},
			expected: &envoycorev3.HealthCheck{
				Timeout:            durationpb.New(5 * time.Second),
				Interval:           durationpb.New(10 * time.Second),
				UnhealthyThreshold: &wrapperspb.UInt32Value{Value: 3},
				HealthyThreshold:   &wrapperspb.UInt32Value{Value: 2},
				HealthChecker: &envoycorev3.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &envoycorev3.HealthCheck_HttpHealthCheck{
						Path: "/health",
					},
				},
			},
		},
		{
			name: "HTTP health check",
			config: &kgateway.HealthCheck{
				Timeout:            metav1.Duration{Duration: 5 * time.Second},
				Interval:           metav1.Duration{Duration: 10 * time.Second},
				UnhealthyThreshold: 3,
				HealthyThreshold:   2,
				Http: &kgateway.HealthCheckHttp{
					Host:   new("example.com"),
					Path:   "/health",
					Method: new("GET"),
				},
			},
			expected: &envoycorev3.HealthCheck{
				Timeout:            durationpb.New(5 * time.Second),
				Interval:           durationpb.New(10 * time.Second),
				UnhealthyThreshold: &wrapperspb.UInt32Value{Value: 3},
				HealthyThreshold:   &wrapperspb.UInt32Value{Value: 2},
				HealthChecker: &envoycorev3.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &envoycorev3.HealthCheck_HttpHealthCheck{
						Host:   "example.com",
						Path:   "/health",
						Method: envoycorev3.RequestMethod_GET,
					},
				},
			},
		},
		{
			name: "gRPC health check",
			config: &kgateway.HealthCheck{
				Timeout:            metav1.Duration{Duration: 5 * time.Second},
				Interval:           metav1.Duration{Duration: 10 * time.Second},
				UnhealthyThreshold: 4,
				HealthyThreshold:   1,
				Grpc: &kgateway.HealthCheckGrpc{
					ServiceName: new("grpc.health.v1.Health"),
					Authority:   new("example.com"),
				},
			},
			expected: &envoycorev3.HealthCheck{
				Timeout:            durationpb.New(5 * time.Second),
				Interval:           durationpb.New(10 * time.Second),
				UnhealthyThreshold: &wrapperspb.UInt32Value{Value: 4},
				HealthyThreshold:   &wrapperspb.UInt32Value{Value: 1},
				HealthChecker: &envoycorev3.HealthCheck_GrpcHealthCheck_{
					GrpcHealthCheck: &envoycorev3.HealthCheck_GrpcHealthCheck{
						ServiceName: "grpc.health.v1.Health",
						Authority:   "example.com",
					},
				},
			},
		},
		{
			name: "HTTP health check with multiple status ranges",
			config: &kgateway.HealthCheck{
				Timeout:            metav1.Duration{Duration: 5 * time.Second},
				Interval:           metav1.Duration{Duration: 10 * time.Second},
				UnhealthyThreshold: 2,
				HealthyThreshold:   3,
				Http: &kgateway.HealthCheckHttp{
					Host: new("example.com"),
					Path: "/health",
				},
			},
			expected: &envoycorev3.HealthCheck{
				Timeout:            durationpb.New(5 * time.Second),
				Interval:           durationpb.New(10 * time.Second),
				UnhealthyThreshold: &wrapperspb.UInt32Value{Value: 2},
				HealthyThreshold:   &wrapperspb.UInt32Value{Value: 3},
				HealthChecker: &envoycorev3.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &envoycorev3.HealthCheck_HttpHealthCheck{
						Host: "example.com",
						Path: "/health",
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

// Omitting NoTrafficInterval preserves Envoy's default regardless of Interval.
func TestHealthCheckNoTrafficIntervalDefault(t *testing.T) {
	for _, interval := range []time.Duration{250 * time.Millisecond, time.Second, 90 * time.Second} {
		t.Run(interval.String(), func(t *testing.T) {
			hc := &kgateway.HealthCheck{
				Timeout:            metav1.Duration{Duration: time.Second},
				Interval:           metav1.Duration{Duration: interval},
				UnhealthyThreshold: 1,
				HealthyThreshold:   1,
				Http:               &kgateway.HealthCheckHttp{Path: "/health"},
			}
			got := translateHealthCheck(hc)
			if got.GetNoTrafficInterval() != nil {
				t.Fatalf("want Envoy default, got no-traffic interval %v", got.GetNoTrafficInterval())
			}
			if got.GetNoTrafficHealthyInterval() != nil {
				t.Fatal("healthy hosts must inherit the same no-traffic interval")
			}
		})
	}
}

func TestHealthCheckNoTrafficIntervalOverride(t *testing.T) {
	for _, protocol := range []string{"http", "grpc"} {
		for _, interval := range []time.Duration{250 * time.Millisecond, time.Second, 90 * time.Second} {
			t.Run(protocol+"/"+interval.String(), func(t *testing.T) {
				hc := &kgateway.HealthCheck{
					Interval:          metav1.Duration{Duration: 10 * time.Second},
					NoTrafficInterval: &metav1.Duration{Duration: interval},
				}
				if protocol == "http" {
					hc.Http = &kgateway.HealthCheckHttp{Path: "/health"}
				} else {
					hc.Grpc = &kgateway.HealthCheckGrpc{}
				}
				got := translateHealthCheck(hc)
				if !proto.Equal(got.GetNoTrafficInterval(), durationpb.New(interval)) {
					t.Fatalf("want no-traffic interval %s, got %v", interval, got.GetNoTrafficInterval())
				}
				if !proto.Equal(got.GetInterval(), durationpb.New(10*time.Second)) {
					t.Fatalf("regular interval changed: %v", got.GetInterval())
				}
				if got.GetNoTrafficHealthyInterval() != nil {
					t.Fatal("healthy hosts must inherit the same no-traffic interval")
				}
			})
		}
	}
}
