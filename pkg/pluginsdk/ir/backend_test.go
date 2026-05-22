package ir

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{
			name: "backends with different gateway backend client certificates should not be equal",
			backend1: func() BackendObjectIR {
				backend := createTestBackendObjectIR(wellknown.TrafficDistributionAny)
				backend.GatewayBackendClientCertificate = &GatewayBackendClientCertificateIR{
					Certificate: TLSCertificate{
						CertChain:  []byte("cert-a"),
						PrivateKey: []byte("key-a"),
					},
				}
				return backend
			},
			backend2: func() BackendObjectIR {
				backend := createTestBackendObjectIR(wellknown.TrafficDistributionAny)
				backend.GatewayBackendClientCertificate = &GatewayBackendClientCertificateIR{
					Certificate: TLSCertificate{
						CertChain:  []byte("cert-b"),
						PrivateKey: []byte("key-b"),
					},
				}
				return backend
			},
			want: false,
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

func TestBackendObjectIRClusterName(t *testing.T) {
	base := createTestBackendObjectIR(wellknown.TrafficDistributionAny)
	base.Kind = "Service"
	base.resourceName = BackendResourceName(base.ObjectSource, base.Port, base.ExtraKey)

	t.Run("keeps the same name when BackendTLSPolicy is attached", func(t *testing.T) {
		withPolicy := base
		withPolicy.AttachedPolicies = AttachedPolicies{
			Policies: map[schema.GroupKind][]PolicyAtt{
				wellknown.BackendTLSPolicyGVK.GroupKind(): {{
					GroupKind: wellknown.BackendTLSPolicyGVK.GroupKind(),
					PolicyRef: &AttachedPolicyRef{
						Group:     wellknown.BackendTLSPolicyGVK.Group,
						Kind:      wellknown.BackendTLSPolicyGVK.Kind,
						Namespace: "default",
						Name:      "backend-tls",
					},
				}},
			},
		}

		assert.Equal(t, base.ClusterName(), withPolicy.ClusterName())
	})
}

func TestBackendObjectIRSetGvPrefixUpdatesClusterName(t *testing.T) {
	backend := NewBackendObjectIR(ObjectSource{
		Namespace: "default",
		Name:      "test-service",
		Kind:      "Service",
	}, 8080, "")

	// Before SetGvPrefix the cached name uses the lowercased Kind as prefix.
	assert.Equal(t, "service_default_test-service_8080", backend.ClusterName())

	backend.SetGvPrefix("kube")

	assert.Equal(t, "kube_default_test-service_8080", backend.ClusterName())
}

func TestBackendObjectIRCloneRecomputesClusterName(t *testing.T) {
	backend := NewBackendObjectIR(ObjectSource{
		Namespace: "ns",
		Name:      "svc",
		Kind:      "Service",
	}, 80, "")
	backend.SetGvPrefix("kube")

	original := backend.ClusterName()
	clone := backend.CloneForGatewayBackendClientCertificate(
		ObjectSource{Group: "gateway.networking.k8s.io", Kind: "Gateway", Namespace: "gwns", Name: "gw"},
		&GatewayBackendClientCertificateIR{},
	)

	assert.NotEqual(t, original, clone.ClusterName(), "clone should have a distinct cluster name after ExtraKey change")
	assert.Contains(t, clone.ClusterName(), "gw_backend_client_cert_gwns_gw")
}

func TestGatewayBackendClientCertificateIRMarshalJSONRedactsCertificate(t *testing.T) {
	clientCertificate := GatewayBackendClientCertificateIR{
		Certificate: TLSCertificate{
			CertChain:  []byte("gateway-cert"),
			PrivateKey: []byte("gateway-key"),
		},
	}

	marshaled, err := json.Marshal(clientCertificate)
	require.NoError(t, err)

	assert.JSONEq(t, `{"certificate":"[REDACTED]"}`, string(marshaled))
	assert.NotContains(t, string(marshaled), "gateway-cert")
	assert.NotContains(t, string(marshaled), "gateway-key")
}
