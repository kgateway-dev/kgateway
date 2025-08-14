package policy

import (
	"testing"
	"time"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func TestBuildRetryPolicy(t *testing.T) {
	tests := []struct {
		name  string
		input *v1alpha1.Retry
		want  *envoyroutev3.RetryPolicy
	}{
		{
			name:  "nil input returns nil",
			input: nil,
			want:  nil,
		},
		{
			name: "basic retry policy with retryOn conditions only",
			input: &v1alpha1.Retry{
				RetryOn:  []v1alpha1.RetryOnCondition{"5xx", "gateway-error", "reset"},
				Attempts: int32(3),
			},
			want: &envoyroutev3.RetryPolicy{
				RetryOn:              "5xx,gateway-error,reset",
				NumRetries:           wrapperspb.UInt32(3),
				RetriableStatusCodes: nil,
			},
		},
		{
			name: "retry policy with status codes without retriable-status-codes in retryOn",
			input: &v1alpha1.Retry{
				RetryOn:     []v1alpha1.RetryOnCondition{"connect-failure", "unavailable"},
				Attempts:    int32(2),
				StatusCodes: []gwv1.HTTPRouteRetryStatusCode{500, 502, 503},
			},
			want: &envoyroutev3.RetryPolicy{
				RetryOn:              "connect-failure,retriable-status-codes,unavailable",
				NumRetries:           wrapperspb.UInt32(2),
				RetriableStatusCodes: []uint32{500, 502, 503},
			},
		},
		{
			name: "retry policy with status codes with retriable-status-codes in retryOn",
			input: &v1alpha1.Retry{
				RetryOn:     []v1alpha1.RetryOnCondition{"connect-failure", "unavailable", "retriable-status-codes"},
				Attempts:    int32(2),
				StatusCodes: []gwv1.HTTPRouteRetryStatusCode{500, 502, 503},
			},
			want: &envoyroutev3.RetryPolicy{
				RetryOn:              "connect-failure,retriable-status-codes,unavailable",
				NumRetries:           wrapperspb.UInt32(2),
				RetriableStatusCodes: []uint32{500, 502, 503},
			},
		},
		{
			name: "retry policy with per-try timeout",
			input: &v1alpha1.Retry{
				RetryOn:       []v1alpha1.RetryOnCondition{"reset", "envoy-ratelimited"},
				Attempts:      int32(5),
				PerTryTimeout: &metav1.Duration{Duration: 2 * time.Second},
			},
			want: &envoyroutev3.RetryPolicy{
				RetryOn:              "envoy-ratelimited,reset",
				NumRetries:           wrapperspb.UInt32(5),
				RetriableStatusCodes: nil,
				PerTryTimeout:        durationpb.New(2 * time.Second),
			},
		},
		{
			name: "retry policy with backoff base interval",
			input: &v1alpha1.Retry{
				RetryOn:             []v1alpha1.RetryOnCondition{"retriable-4xx"},
				Attempts:            int32(4),
				BackoffBaseInterval: &metav1.Duration{Duration: 100 * time.Millisecond},
			},
			want: &envoyroutev3.RetryPolicy{
				RetryOn:              "retriable-4xx",
				NumRetries:           wrapperspb.UInt32(4),
				RetriableStatusCodes: nil,
				RetryBackOff: &envoyroutev3.RetryPolicy_RetryBackOff{
					BaseInterval: durationpb.New(100 * time.Millisecond),
				},
			},
		},
		{
			name: "comprehensive retry policy with all fields",
			input: &v1alpha1.Retry{
				RetryOn:             []v1alpha1.RetryOnCondition{"cancelled", "deadline-exceeded", "internal"},
				Attempts:            int32(10),
				StatusCodes:         []gwv1.HTTPRouteRetryStatusCode{401, 403, 429},
				PerTryTimeout:       &metav1.Duration{Duration: 5 * time.Second},
				BackoffBaseInterval: &metav1.Duration{Duration: 25 * time.Millisecond},
			},
			want: &envoyroutev3.RetryPolicy{
				RetryOn:              "cancelled,deadline-exceeded,internal,retriable-status-codes",
				NumRetries:           wrapperspb.UInt32(10),
				RetriableStatusCodes: []uint32{401, 403, 429},
				PerTryTimeout:        durationpb.New(5 * time.Second),
				RetryBackOff: &envoyroutev3.RetryPolicy_RetryBackOff{
					BaseInterval: durationpb.New(25 * time.Millisecond),
				},
			},
		},
		{
			name: "retry policy with only status codes (no retryOn)",
			input: &v1alpha1.Retry{
				Attempts:    int32(1),
				StatusCodes: []gwv1.HTTPRouteRetryStatusCode{404, 408},
			},
			want: &envoyroutev3.RetryPolicy{
				RetryOn:              "retriable-status-codes",
				NumRetries:           wrapperspb.UInt32(1),
				RetriableStatusCodes: []uint32{404, 408},
			},
		},
		{
			name: "retry policy with duplicate retryOn conditions (should be deduplicated)",
			input: &v1alpha1.Retry{
				RetryOn:  []v1alpha1.RetryOnCondition{"5xx", "gateway-error", "5xx", "reset", "gateway-error"},
				Attempts: int32(3),
			},
			want: &envoyroutev3.RetryPolicy{
				RetryOn:              "5xx,gateway-error,reset",
				NumRetries:           wrapperspb.UInt32(3),
				RetriableStatusCodes: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)
			got := BuildRetryPolicy(tt.input)
			diff := cmp.Diff(got, tt.want, protocmp.Transform())
			a.Empty(diff)
		})
	}
}
