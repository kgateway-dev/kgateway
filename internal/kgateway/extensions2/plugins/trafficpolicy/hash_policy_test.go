package trafficpolicy

import (
	"testing"
	"time"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/durationpb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func TestHashPolicyForSpec(t *testing.T) {
	tests := []struct {
		name     string
		spec     v1alpha1.TrafficPolicySpec
		expected []*envoyroutev3.RouteAction_HashPolicy
	}{
		{
			name: "nil hash policies",
			spec: v1alpha1.TrafficPolicySpec{
				HashPolicies: nil,
			},
			expected: nil,
		},
		{
			name: "empty hash policies",
			spec: v1alpha1.TrafficPolicySpec{
				HashPolicies: []*v1alpha1.HashPolicy{},
			},
			expected: nil,
		},
		{
			name: "header hash policy",
			spec: v1alpha1.TrafficPolicySpec{
				HashPolicies: []*v1alpha1.HashPolicy{
					{
						Header: &v1alpha1.Header{
							Name: "x-user-id",
						},
						Terminal: ptr.To(true),
					},
				},
			},
			expected: []*envoyroutev3.RouteAction_HashPolicy{
				{
					Terminal: true,
					PolicySpecifier: &envoyroutev3.RouteAction_HashPolicy_Header_{
						Header: &envoyroutev3.RouteAction_HashPolicy_Header{
							HeaderName: "x-user-id",
						},
					},
				},
			},
		},
		{
			name: "cookie hash policy without TTL and path",
			spec: v1alpha1.TrafficPolicySpec{
				HashPolicies: []*v1alpha1.HashPolicy{
					{
						Cookie: &v1alpha1.Cookie{
							Name: "session-id",
						},
					},
				},
			},
			expected: []*envoyroutev3.RouteAction_HashPolicy{
				{
					Terminal: false,
					PolicySpecifier: &envoyroutev3.RouteAction_HashPolicy_Cookie_{
						Cookie: &envoyroutev3.RouteAction_HashPolicy_Cookie{
							Name: "session-id",
						},
					},
				},
			},
		},
		{
			name: "cookie hash policy with TTL and path",
			spec: v1alpha1.TrafficPolicySpec{
				HashPolicies: []*v1alpha1.HashPolicy{
					{
						Cookie: &v1alpha1.Cookie{
							Name: "session-id",
							TTL: &metav1.Duration{
								Duration: 30 * time.Minute,
							},
							Path: ptr.To("/api"),
							Attributes: map[string]string{
								"domain": "example.com",
								"secure": "true",
							},
						},
						Terminal: ptr.To(true),
					},
				},
			},
			expected: []*envoyroutev3.RouteAction_HashPolicy{
				{
					Terminal: true,
					PolicySpecifier: &envoyroutev3.RouteAction_HashPolicy_Cookie_{
						Cookie: &envoyroutev3.RouteAction_HashPolicy_Cookie{
							Name: "session-id",
							Ttl:  durationpb.New(30 * time.Minute),
							Path: "/api",
							Attributes: []*envoyroutev3.RouteAction_HashPolicy_CookieAttribute{
								{
									Name:  "domain",
									Value: "example.com",
								},
								{
									Name:  "secure",
									Value: "true",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "source IP hash policy",
			spec: v1alpha1.TrafficPolicySpec{
				HashPolicies: []*v1alpha1.HashPolicy{
					{
						SourceIP: &v1alpha1.SourceIP{},
						Terminal: ptr.To(false),
					},
				},
			},
			expected: []*envoyroutev3.RouteAction_HashPolicy{
				{
					Terminal: false,
					PolicySpecifier: &envoyroutev3.RouteAction_HashPolicy_ConnectionProperties_{
						ConnectionProperties: &envoyroutev3.RouteAction_HashPolicy_ConnectionProperties{
							SourceIp: true,
						},
					},
				},
			},
		},
		{
			name: "multiple hash policies",
			spec: v1alpha1.TrafficPolicySpec{
				HashPolicies: []*v1alpha1.HashPolicy{
					{
						Header: &v1alpha1.Header{
							Name: "x-user-id",
						},
						Terminal: ptr.To(true),
					},
					{
						Cookie: &v1alpha1.Cookie{
							Name: "session-id",
							TTL: &metav1.Duration{
								Duration: 1 * time.Hour,
							},
						},
					},
					{
						SourceIP: &v1alpha1.SourceIP{},
					},
				},
			},
			expected: []*envoyroutev3.RouteAction_HashPolicy{
				{
					Terminal: true,
					PolicySpecifier: &envoyroutev3.RouteAction_HashPolicy_Header_{
						Header: &envoyroutev3.RouteAction_HashPolicy_Header{
							HeaderName: "x-user-id",
						},
					},
				},
				{
					Terminal: false,
					PolicySpecifier: &envoyroutev3.RouteAction_HashPolicy_Cookie_{
						Cookie: &envoyroutev3.RouteAction_HashPolicy_Cookie{
							Name: "session-id",
							Ttl:  durationpb.New(1 * time.Hour),
						},
					},
				},
				{
					Terminal: false,
					PolicySpecifier: &envoyroutev3.RouteAction_HashPolicy_ConnectionProperties_{
						ConnectionProperties: &envoyroutev3.RouteAction_HashPolicy_ConnectionProperties{
							SourceIp: true,
						},
					},
				},
			},
		},
		{
			name: "cookie hash policy with nil TTL",
			spec: v1alpha1.TrafficPolicySpec{
				HashPolicies: []*v1alpha1.HashPolicy{
					{
						Cookie: &v1alpha1.Cookie{
							Name: "session-id",
							TTL:  nil,
							Path: ptr.To("/api"),
						},
						Terminal: ptr.To(false),
					},
				},
			},
			expected: []*envoyroutev3.RouteAction_HashPolicy{
				{
					Terminal: false,
					PolicySpecifier: &envoyroutev3.RouteAction_HashPolicy_Cookie_{
						Cookie: &envoyroutev3.RouteAction_HashPolicy_Cookie{
							Name: "session-id",
							Path: "/api",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outSpec := &trafficPolicySpecIr{}

			hashPolicyForSpec(tt.spec, outSpec)

			assert.Equal(t, tt.expected, outSpec.hashPolicies)
		})
	}
}
