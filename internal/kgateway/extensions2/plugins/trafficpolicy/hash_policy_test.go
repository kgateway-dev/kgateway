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

func TestHashPolicyIREquals(t *testing.T) {
	createSimpleHashPolicies := func(headerName string) []*envoyroutev3.RouteAction_HashPolicy {
		return []*envoyroutev3.RouteAction_HashPolicy{
			{
				PolicySpecifier: &envoyroutev3.RouteAction_HashPolicy_Header_{
					Header: &envoyroutev3.RouteAction_HashPolicy_Header{
						HeaderName: headerName,
					},
				},
				Terminal: false,
			},
		}
	}
	createHashPoliciesWithTerminal := func(terminal bool) []*envoyroutev3.RouteAction_HashPolicy {
		return []*envoyroutev3.RouteAction_HashPolicy{
			{
				PolicySpecifier: &envoyroutev3.RouteAction_HashPolicy_Header_{
					Header: &envoyroutev3.RouteAction_HashPolicy_Header{
						HeaderName: "x-user-id",
					},
				},
				Terminal: terminal,
			},
		}
	}

	tests := []struct {
		name     string
		hash1    *hashPolicyIR
		hash2    *hashPolicyIR
		expected bool
	}{
		{
			name:     "both nil are equal",
			hash1:    nil,
			hash2:    nil,
			expected: true,
		},
		{
			name:     "nil vs non-nil are not equal",
			hash1:    nil,
			hash2:    &hashPolicyIR{policies: createSimpleHashPolicies("x-user-id")},
			expected: false,
		},
		{
			name:     "non-nil vs nil are not equal",
			hash1:    &hashPolicyIR{policies: createSimpleHashPolicies("x-user-id")},
			hash2:    nil,
			expected: false,
		},
		{
			name:     "same instance is equal",
			hash1:    &hashPolicyIR{policies: createSimpleHashPolicies("x-user-id")},
			hash2:    &hashPolicyIR{policies: createSimpleHashPolicies("x-user-id")},
			expected: true,
		},
		{
			name:     "different header names are not equal",
			hash1:    &hashPolicyIR{policies: createSimpleHashPolicies("x-user-id")},
			hash2:    &hashPolicyIR{policies: createSimpleHashPolicies("x-session-id")},
			expected: false,
		},
		{
			name:     "different terminal settings are not equal",
			hash1:    &hashPolicyIR{policies: createHashPoliciesWithTerminal(true)},
			hash2:    &hashPolicyIR{policies: createHashPoliciesWithTerminal(false)},
			expected: false,
		},
		{
			name:     "same terminal settings are equal",
			hash1:    &hashPolicyIR{policies: createHashPoliciesWithTerminal(true)},
			hash2:    &hashPolicyIR{policies: createHashPoliciesWithTerminal(true)},
			expected: true,
		},
		{
			name:     "nil hash policy fields are equal",
			hash1:    &hashPolicyIR{policies: nil},
			hash2:    &hashPolicyIR{policies: nil},
			expected: true,
		},
		{
			name:     "nil vs non-nil hash policy fields are not equal",
			hash1:    &hashPolicyIR{policies: nil},
			hash2:    &hashPolicyIR{policies: createSimpleHashPolicies("x-test")},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.hash1.Equals(tt.hash2)
			assert.Equal(t, tt.expected, result)

			// Test symmetry: a.Equals(b) should equal b.Equals(a)
			reverseResult := tt.hash2.Equals(tt.hash1)
			assert.Equal(t, result, reverseResult, "Equals should be symmetric")
		})
	}

	// Test reflexivity: x.Equals(x) should always be true for non-nil values
	t.Run("reflexivity", func(t *testing.T) {
		hash := &hashPolicyIR{policies: createSimpleHashPolicies("x-test")}
		assert.True(t, hash.Equals(hash), "hash should equal itself")
	})

	// Test transitivity: if a.Equals(b) && b.Equals(c), then a.Equals(c)
	t.Run("transitivity", func(t *testing.T) {
		createSameHash := func() *hashPolicyIR {
			return &hashPolicyIR{policies: createHashPoliciesWithTerminal(false)}
		}

		a := createSameHash()
		b := createSameHash()
		c := createSameHash()

		assert.True(t, a.Equals(b), "a should equal b")
		assert.True(t, b.Equals(c), "b should equal c")
		assert.True(t, a.Equals(c), "a should equal c (transitivity)")
	})
}

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
			constructHashPolicy(tt.spec, outSpec)

			var actual []*envoyroutev3.RouteAction_HashPolicy
			if outSpec.hashPolicies != nil {
				actual = outSpec.hashPolicies.policies
			}
			assert.Equal(t, tt.expected, actual)
		})
	}
}
