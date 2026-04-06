package ir

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

func TestParseAppProtocol(t *testing.T) {
	tests := []struct {
		name     string
		input    *string
		expected AppProtocol
	}{
		{
			name:     "http2",
			input:    new("http2"),
			expected: HTTP2AppProtocol,
		},
		{
			name:     "grpc",
			input:    new("grpc"),
			expected: HTTP2AppProtocol,
		},
		{
			name:     "grpc-web",
			input:    new("grpc-web"),
			expected: HTTP2AppProtocol,
		},
		{
			name:     "kubernetes.io/h2c",
			input:    new("kubernetes.io/h2c"),
			expected: HTTP2AppProtocol,
		},
		{
			name:     "kubernetes.io/ws",
			input:    new("kubernetes.io/ws"),
			expected: WebSocketAppProtocol,
		},
		{
			name:     "HTTP2",
			input:    new("HTTP2"),
			expected: HTTP2AppProtocol,
		},
		{
			name:     "(empty)",
			input:    nil,
			expected: DefaultAppProtocol,
		},
		{
			name:     "unknown",
			input:    new("unknown"),
			expected: DefaultAppProtocol,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)
			actual := ParseAppProtocol(tt.input)
			a.Equal(tt.expected, actual)
		})
	}
}

func createTestBackendObjectIR(trafficDist wellknown.TrafficDistribution) BackendObjectIR {
	return BackendObjectIR{
		ObjectSource: ObjectSource{
			Namespace: "default",
			Name:      "test-service",
			Group:     "",
			Kind:      "Service",
		},
		Port: 8080,
		Obj: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "test-service",
				Namespace:       "default",
				UID:             "test-uid",
				ResourceVersion: "1",
				Generation:      1,
			},
		},
		TrafficDistribution: trafficDist,
	}
}

func TestBackendObjectIREquals(t *testing.T) {
	tests := []struct {
		name     string
		backend1 func() BackendObjectIR
		backend2 func() BackendObjectIR
		want     bool
	}{
		{
			name:     "same backend objects should be equal",
			backend1: func() BackendObjectIR { return createTestBackendObjectIR(wellknown.TrafficDistributionAny) },
			backend2: func() BackendObjectIR { return createTestBackendObjectIR(wellknown.TrafficDistributionAny) },
			want:     true,
		},
		{
			name:     "backends with different traffic distribution should not be equal",
			backend1: func() BackendObjectIR { return createTestBackendObjectIR(wellknown.TrafficDistributionAny) },
			backend2: func() BackendObjectIR { return createTestBackendObjectIR(wellknown.TrafficDistributionPreferSameZone) },
			want:     false,
		},
		{
			name:     "backends with different traffic distribution PreferSameZone vs PreferNetwork should not be equal",
			backend1: func() BackendObjectIR { return createTestBackendObjectIR(wellknown.TrafficDistributionPreferSameZone) },
			backend2: func() BackendObjectIR { return createTestBackendObjectIR(wellknown.TrafficDistributionPreferNetwork) },
			want:     false,
		},
		{
			name:     "backends with different traffic distribution PreferSameNode vs PreferNetwork should not be equal",
			backend1: func() BackendObjectIR { return createTestBackendObjectIR(wellknown.TrafficDistributionPreferSameNode) },
			backend2: func() BackendObjectIR { return createTestBackendObjectIR(wellknown.TrafficDistributionPreferNetwork) },
			want:     false,
		},
		{
			name:     "backends with same PreferNetwork traffic distribution should be equal",
			backend1: func() BackendObjectIR { return createTestBackendObjectIR(wellknown.TrafficDistributionPreferNetwork) },
			backend2: func() BackendObjectIR { return createTestBackendObjectIR(wellknown.TrafficDistributionPreferNetwork) },
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)
			backend1 := tt.backend1()
			backend2 := tt.backend2()

			// Test forward equality
			result := backend1.Equals(backend2)
			a.Equal(tt.want, result, "BackendObjectIR.Equals() result mismatch")

			// Test symmetry: a.Equals(b) should equal b.Equals(a)
			reverseResult := backend2.Equals(backend1)
			a.Equal(result, reverseResult, "symmetry check failed: a.Equals(b) != b.Equals(a)")

			// Test reflexivity: x.Equals(x) should always be true
			a.True(backend1.Equals(backend1), "reflexivity check failed for backend1")
			a.True(backend2.Equals(backend2), "reflexivity check failed for backend2")
		})
	}
}

type testBackendPolicyIR struct {
	created time.Time
}

func (t *testBackendPolicyIR) CreationTime() time.Time {
	return t.created
}

func (t *testBackendPolicyIR) Equals(other any) bool {
	o, ok := other.(*testBackendPolicyIR)
	if !ok {
		return false
	}
	return t.created.Equal(o.created)
}

func TestBackendObjectIRClusterName(t *testing.T) {
	base := createTestBackendObjectIR(wellknown.TrafficDistributionAny)
	base.Kind = "Service"
	base.resourceName = BackendResourceName(base.ObjectSource, base.Port, base.ExtraKey)

	t.Run("does not rotate without BackendTLSPolicy", func(t *testing.T) {
		assert.Equal(t, "service_default_test-service_8080", base.ClusterName())
	})

	t.Run("rotates when BackendTLSPolicy is attached", func(t *testing.T) {
		withPolicy := base
		withPolicy.AttachedPolicies = AttachedPolicies{
			Policies: map[schema.GroupKind][]PolicyAtt{
				wellknown.BackendTLSPolicyGVK.GroupKind(): {
					{
						Generation: 1,
						GroupKind:  wellknown.BackendTLSPolicyGVK.GroupKind(),
						PolicyIr:   &testBackendPolicyIR{created: time.Unix(10, 0)},
						PolicyRef: &AttachedPolicyRef{
							Group:     wellknown.BackendTLSPolicyGVK.Group,
							Kind:      wellknown.BackendTLSPolicyGVK.Kind,
							Namespace: "default",
							Name:      "backend-tls",
						},
					},
				},
			},
		}

		assert.NotEqual(t, base.ClusterName(), withPolicy.ClusterName())
		assert.Contains(t, withPolicy.ClusterName(), "btls")
	})

	t.Run("keeps the same cluster name when only a conflicting loser changes", func(t *testing.T) {
		older := PolicyAtt{
			Generation: 1,
			GroupKind:  wellknown.BackendTLSPolicyGVK.GroupKind(),
			PolicyIr:   &testBackendPolicyIR{created: time.Unix(10, 0)},
			PolicyRef: &AttachedPolicyRef{
				Group:     wellknown.BackendTLSPolicyGVK.Group,
				Kind:      wellknown.BackendTLSPolicyGVK.Kind,
				Namespace: "default",
				Name:      "older",
			},
		}
		newer := PolicyAtt{
			Generation: 1,
			GroupKind:  wellknown.BackendTLSPolicyGVK.GroupKind(),
			PolicyIr:   &testBackendPolicyIR{created: time.Unix(20, 0)},
			PolicyRef: &AttachedPolicyRef{
				Group:     wellknown.BackendTLSPolicyGVK.Group,
				Kind:      wellknown.BackendTLSPolicyGVK.Kind,
				Namespace: "default",
				Name:      "newer",
			},
		}

		onlyWinner := base
		onlyWinner.AttachedPolicies = AttachedPolicies{
			Policies: map[schema.GroupKind][]PolicyAtt{
				wellknown.BackendTLSPolicyGVK.GroupKind(): {older},
			},
		}
		withConflictingLoser := base
		withConflictingLoser.AttachedPolicies = AttachedPolicies{
			Policies: map[schema.GroupKind][]PolicyAtt{
				wellknown.BackendTLSPolicyGVK.GroupKind(): {older, newer},
			},
		}

		assert.Equal(t, onlyWinner.ClusterName(), withConflictingLoser.ClusterName())
	})
}
