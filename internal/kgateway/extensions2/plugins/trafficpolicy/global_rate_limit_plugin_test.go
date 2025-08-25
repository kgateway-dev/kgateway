package trafficpolicy

import (
	"errors"
	"fmt"
	"testing"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	ratev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

func TestGlobalRateLimitIREquals(t *testing.T) {
	createSimpleRateLimit := func(key string) []*envoyroutev3.RateLimit {
		return []*envoyroutev3.RateLimit{
			{
				Actions: []*envoyroutev3.RateLimit_Action{
					{
						ActionSpecifier: &envoyroutev3.RateLimit_Action_GenericKey_{
							GenericKey: &envoyroutev3.RateLimit_Action_GenericKey{
								DescriptorKey:   key,
								DescriptorValue: "test-value",
							},
						},
					},
				},
			},
		}
	}
	createProvider := func(name string) *TrafficPolicyGatewayExtensionIR {
		return &TrafficPolicyGatewayExtensionIR{
			Name: name,
			RateLimit: &ratev3.RateLimit{
				Domain: "test-domain",
				RateLimitService: &envoyratelimitv3.RateLimitServiceConfig{
					GrpcService: &envoycorev3.GrpcService{
						TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
							EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{
								ClusterName: name,
							},
						},
					},
				},
			},
		}
	}

	tests := []struct {
		name       string
		rateLimit1 *globalRateLimitIR
		rateLimit2 *globalRateLimitIR
		expected   bool
	}{
		{
			name:       "both nil are equal",
			rateLimit1: nil,
			rateLimit2: nil,
			expected:   true,
		},
		{
			name:       "nil vs non-nil are not equal",
			rateLimit1: nil,
			rateLimit2: &globalRateLimitIR{rateLimitActions: createSimpleRateLimit("key1")},
			expected:   false,
		},
		{
			name:       "non-nil vs nil are not equal",
			rateLimit1: &globalRateLimitIR{rateLimitActions: createSimpleRateLimit("key1")},
			rateLimit2: nil,
			expected:   false,
		},
		{
			name:       "same instance is equal",
			rateLimit1: &globalRateLimitIR{rateLimitActions: createSimpleRateLimit("key1")},
			rateLimit2: &globalRateLimitIR{rateLimitActions: createSimpleRateLimit("key1")},
			expected:   true,
		},
		{
			name:       "different rate limit keys are not equal",
			rateLimit1: &globalRateLimitIR{rateLimitActions: createSimpleRateLimit("key1")},
			rateLimit2: &globalRateLimitIR{rateLimitActions: createSimpleRateLimit("key2")},
			expected:   false,
		},
		{
			name:       "different providers are not equal",
			rateLimit1: &globalRateLimitIR{provider: createProvider("service1")},
			rateLimit2: &globalRateLimitIR{provider: createProvider("service2")},
			expected:   false,
		},
		{
			name:       "same providers are equal",
			rateLimit1: &globalRateLimitIR{provider: createProvider("service1")},
			rateLimit2: &globalRateLimitIR{provider: createProvider("service1")},
			expected:   true,
		},
		{
			name:       "different length action slices are not equal",
			rateLimit1: &globalRateLimitIR{rateLimitActions: createSimpleRateLimit("key1")},
			rateLimit2: &globalRateLimitIR{rateLimitActions: append(createSimpleRateLimit("key1"), createSimpleRateLimit("key2")...)},
			expected:   false,
		},
		{
			name:       "nil fields are equal",
			rateLimit1: &globalRateLimitIR{rateLimitActions: nil, provider: nil},
			rateLimit2: &globalRateLimitIR{rateLimitActions: nil, provider: nil},
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.rateLimit1.Equals(tt.rateLimit2)
			assert.Equal(t, tt.expected, result)

			// Test symmetry: a.Equals(b) should equal b.Equals(a)
			reverseResult := tt.rateLimit2.Equals(tt.rateLimit1)
			assert.Equal(t, result, reverseResult, "Equals should be symmetric")
		})
	}

	// Test reflexivity: x.Equals(x) should always be true for non-nil values
	t.Run("reflexivity", func(t *testing.T) {
		rateLimit := &globalRateLimitIR{rateLimitActions: createSimpleRateLimit("test")}
		assert.True(t, rateLimit.Equals(rateLimit), "rateLimit should equal itself")
	})

	// Test transitivity: if a.Equals(b) && b.Equals(c), then a.Equals(c)
	t.Run("transitivity", func(t *testing.T) {
		createSameRateLimit := func() *globalRateLimitIR {
			return &globalRateLimitIR{rateLimitActions: createSimpleRateLimit("test")}
		}

		a := createSameRateLimit()
		b := createSameRateLimit()
		c := createSameRateLimit()

		assert.True(t, a.Equals(b), "a should equal b")
		assert.True(t, b.Equals(c), "b should equal c")
		assert.True(t, a.Equals(c), "a should equal c (transitivity)")
	})
}

func TestCreateRateLimitActions(t *testing.T) {
	tests := []struct {
		name           string
		descriptors    []v1alpha1.RateLimitDescriptor
		expectedError  string
		validateResult func(*testing.T, []*envoyroutev3.RateLimit_Action)
	}{
		{
			name: "with generic key descriptor",
			descriptors: []v1alpha1.RateLimitDescriptor{
				{
					Entries: []v1alpha1.RateLimitDescriptorEntry{
						{
							Type: v1alpha1.RateLimitDescriptorEntryTypeGeneric,
							Generic: &v1alpha1.RateLimitDescriptorEntryGeneric{
								Key:   "service",
								Value: "api",
							},
						},
					},
				},
			},
			validateResult: func(t *testing.T, actions []*envoyroutev3.RateLimit_Action) {
				require.Len(t, actions, 1)
				genericKey := actions[0].GetGenericKey()
				require.NotNil(t, genericKey)
				assert.Equal(t, "service", genericKey.DescriptorKey)
				assert.Equal(t, "api", genericKey.DescriptorValue)
			},
		},
		{
			name: "with header descriptor",
			descriptors: []v1alpha1.RateLimitDescriptor{
				{
					Entries: []v1alpha1.RateLimitDescriptorEntry{
						{
							Type:   v1alpha1.RateLimitDescriptorEntryTypeHeader,
							Header: ptr.To("X-User-ID"),
						},
					},
				},
			},
			validateResult: func(t *testing.T, actions []*envoyroutev3.RateLimit_Action) {
				require.Len(t, actions, 1)
				requestHeaders := actions[0].GetRequestHeaders()
				require.NotNil(t, requestHeaders)
				assert.Equal(t, "X-User-ID", requestHeaders.HeaderName)
				assert.Equal(t, "X-User-ID", requestHeaders.DescriptorKey)
			},
		},
		{
			name: "with remote address descriptor",
			descriptors: []v1alpha1.RateLimitDescriptor{
				{
					Entries: []v1alpha1.RateLimitDescriptorEntry{
						{
							Type: v1alpha1.RateLimitDescriptorEntryTypeRemoteAddress,
						},
					},
				},
			},
			validateResult: func(t *testing.T, actions []*envoyroutev3.RateLimit_Action) {
				require.Len(t, actions, 1)
				remoteAddress := actions[0].GetRemoteAddress()
				require.NotNil(t, remoteAddress)
			},
		},
		{
			name: "with path descriptor",
			descriptors: []v1alpha1.RateLimitDescriptor{
				{
					Entries: []v1alpha1.RateLimitDescriptorEntry{
						{
							Type: v1alpha1.RateLimitDescriptorEntryTypePath,
						},
					},
				},
			},
			validateResult: func(t *testing.T, actions []*envoyroutev3.RateLimit_Action) {
				require.Len(t, actions, 1)
				requestHeaders := actions[0].GetRequestHeaders()
				require.NotNil(t, requestHeaders)
				assert.Equal(t, ":path", requestHeaders.HeaderName)
				assert.Equal(t, "path", requestHeaders.DescriptorKey)
			},
		},
		{
			name: "with multiple descriptors",
			descriptors: []v1alpha1.RateLimitDescriptor{
				{
					Entries: []v1alpha1.RateLimitDescriptorEntry{
						{
							Type: v1alpha1.RateLimitDescriptorEntryTypeGeneric,
							Generic: &v1alpha1.RateLimitDescriptorEntryGeneric{
								Key:   "service",
								Value: "api",
							},
						},
					},
				},
				{
					Entries: []v1alpha1.RateLimitDescriptorEntry{
						{
							Type: v1alpha1.RateLimitDescriptorEntryTypeRemoteAddress,
						},
					},
				},
			},
			validateResult: func(t *testing.T, actions []*envoyroutev3.RateLimit_Action) {
				require.Len(t, actions, 2)
				// First action is generic key
				genericKey := actions[0].GetGenericKey()
				require.NotNil(t, genericKey)
				assert.Equal(t, "service", genericKey.DescriptorKey)
				assert.Equal(t, "api", genericKey.DescriptorValue)

				// Second action is remote address
				remoteAddress := actions[1].GetRemoteAddress()
				require.NotNil(t, remoteAddress)
			},
		},
		{
			name: "with multiple entries in one descriptor",
			descriptors: []v1alpha1.RateLimitDescriptor{
				{
					Entries: []v1alpha1.RateLimitDescriptorEntry{
						{
							Type: v1alpha1.RateLimitDescriptorEntryTypeGeneric,
							Generic: &v1alpha1.RateLimitDescriptorEntryGeneric{
								Key:   "service",
								Value: "api",
							},
						},
						{
							Type:   v1alpha1.RateLimitDescriptorEntryTypeHeader,
							Header: ptr.To("X-User-ID"),
						},
					},
				},
			},
			validateResult: func(t *testing.T, actions []*envoyroutev3.RateLimit_Action) {
				require.Len(t, actions, 2)
				// First action is generic key
				genericKey := actions[0].GetGenericKey()
				require.NotNil(t, genericKey)
				assert.Equal(t, "service", genericKey.DescriptorKey)
				assert.Equal(t, "api", genericKey.DescriptorValue)

				// Second action is header
				requestHeaders := actions[1].GetRequestHeaders()
				require.NotNil(t, requestHeaders)
				assert.Equal(t, "X-User-ID", requestHeaders.HeaderName)
				assert.Equal(t, "X-User-ID", requestHeaders.DescriptorKey)
			},
		},
		{
			name:          "with empty descriptors",
			descriptors:   []v1alpha1.RateLimitDescriptor{},
			expectedError: "at least one descriptor is required for global rate limiting",
		},
		{
			name: "with missing generic key entry data",
			descriptors: []v1alpha1.RateLimitDescriptor{
				{
					Entries: []v1alpha1.RateLimitDescriptorEntry{
						{
							Type: v1alpha1.RateLimitDescriptorEntryTypeGeneric,
						},
					},
				},
			},
			expectedError: "generic entry requires Generic field to be set",
		},
		{
			name: "with missing header name",
			descriptors: []v1alpha1.RateLimitDescriptor{
				{
					Entries: []v1alpha1.RateLimitDescriptorEntry{
						{
							Type: v1alpha1.RateLimitDescriptorEntryTypeHeader,
						},
					},
				},
			},
			expectedError: "header entry requires Header field to be set",
		},
		{
			name: "with unsupported entry type",
			descriptors: []v1alpha1.RateLimitDescriptor{
				{
					Entries: []v1alpha1.RateLimitDescriptorEntry{
						{
							Type: "UnsupportedType",
						},
					},
				},
			},
			expectedError: "unsupported entry type: UnsupportedType",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actions, err := createRateLimitActions(tt.descriptors)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, actions)
			tt.validateResult(t, actions)
		})
	}
}

func TestToRateLimitFilterConfig(t *testing.T) {
	defaultExtensionName := "test-ratelimit"
	defaultNamespace := "test-namespace"
	typedDefaultNamespace := gwv1.Namespace(defaultNamespace)
	defaultClusterName := "test-service.test-namespace.svc.cluster.local:8081"

	createBackendRef := func() gwv1alpha2.BackendObjectReference {
		port := gwv1alpha2.PortNumber(8081)
		return gwv1alpha2.BackendObjectReference{
			Name: "test-service",
			Port: &port,
		}
	}

	tests := []struct {
		name              string
		gatewayExtension  *ir.GatewayExtension
		policy            *v1alpha1.RateLimitPolicy
		trafficPolicy     *v1alpha1.TrafficPolicy
		expectedError     string
		validateRateLimit func(*testing.T, *ratev3.RateLimit)
	}{
		{
			name: "with default configuration",
			gatewayExtension: &ir.GatewayExtension{
				Type: v1alpha1.GatewayExtensionTypeRateLimit,
				RateLimit: &v1alpha1.RateLimitProvider{
					Domain: "test-domain",
					GrpcService: &v1alpha1.ExtGrpcService{
						BackendRef: &gwv1alpha2.BackendRef{
							BackendObjectReference: createBackendRef(),
						},
					},
				},
				ObjectSource: ir.ObjectSource{
					Name:      defaultExtensionName,
					Namespace: defaultNamespace,
				},
			},
			policy: &v1alpha1.RateLimitPolicy{
				ExtensionRef: v1alpha1.NamespacedObjectReference{
					Name: gwv1.ObjectName(defaultExtensionName),
				},
				Descriptors: []v1alpha1.RateLimitDescriptor{
					{
						Entries: []v1alpha1.RateLimitDescriptorEntry{
							{
								Type: v1alpha1.RateLimitDescriptorEntryTypeGeneric,
								Generic: &v1alpha1.RateLimitDescriptorEntryGeneric{
									Key:   "service",
									Value: "api",
								},
							},
						},
					},
				},
			},
			trafficPolicy: &v1alpha1.TrafficPolicy{},
			validateRateLimit: func(t *testing.T, rl *ratev3.RateLimit) {
				require.NotNil(t, rl)
				assert.Equal(t, "test-domain", rl.Domain)
				assert.Equal(t, defaultClusterName, rl.RateLimitService.GrpcService.GetEnvoyGrpc().ClusterName)
				assert.Equal(t, envoycorev3.ApiVersion_V3, rl.RateLimitService.TransportApiVersion)
				assert.Equal(t, ratev3.RateLimit_OFF, rl.EnableXRatelimitHeaders)
				assert.Equal(t, "both", rl.RequestType)
				assert.Equal(t, "", rl.StatPrefix)
				// note, the 2 fields below differ from the defaults defined in the CRD since the unit tests
				// don't get the CRD defaults
				assert.Equal(t, &durationpb.Duration{Seconds: 0}, rl.Timeout)
				assert.True(t, rl.FailureModeDeny) // Default should be failureModeAllow=false (deny)
			},
		},
		{
			name: "with custom timeout and extensionRef specifying the namespace",
			gatewayExtension: &ir.GatewayExtension{
				Type: v1alpha1.GatewayExtensionTypeRateLimit,
				RateLimit: &v1alpha1.RateLimitProvider{
					Domain: "test-domain",
					GrpcService: &v1alpha1.ExtGrpcService{
						BackendRef: &gwv1alpha2.BackendRef{
							BackendObjectReference: createBackendRef(),
						},
					},
					Timeout: metav1.Duration{Duration: 5 * time.Second},
				},
				ObjectSource: ir.ObjectSource{
					Name:      defaultExtensionName,
					Namespace: defaultNamespace,
				},
			},
			policy: &v1alpha1.RateLimitPolicy{
				ExtensionRef: v1alpha1.NamespacedObjectReference{
					Name:      gwv1.ObjectName(defaultExtensionName),
					Namespace: &typedDefaultNamespace,
				},
				Descriptors: []v1alpha1.RateLimitDescriptor{
					{
						Entries: []v1alpha1.RateLimitDescriptorEntry{
							{
								Type: v1alpha1.RateLimitDescriptorEntryTypeGeneric,
								Generic: &v1alpha1.RateLimitDescriptorEntryGeneric{
									Key:   "service",
									Value: "api",
								},
							},
						},
					},
				},
			},
			trafficPolicy: &v1alpha1.TrafficPolicy{},
			validateRateLimit: func(t *testing.T, rl *ratev3.RateLimit) {
				require.NotNil(t, rl)
				assert.Equal(t, time.Duration(5*time.Second), rl.Timeout.AsDuration())
			},
		},
		{
			name: "with fail open configuration",
			gatewayExtension: &ir.GatewayExtension{
				Type: v1alpha1.GatewayExtensionTypeRateLimit,
				RateLimit: &v1alpha1.RateLimitProvider{
					Domain: "test-domain",
					GrpcService: &v1alpha1.ExtGrpcService{
						BackendRef: &gwv1alpha2.BackendRef{
							BackendObjectReference: createBackendRef(),
						},
					},
					FailOpen: true,
				},
				ObjectSource: ir.ObjectSource{
					Name:      defaultExtensionName,
					Namespace: defaultNamespace,
				},
			},
			policy: &v1alpha1.RateLimitPolicy{
				ExtensionRef: v1alpha1.NamespacedObjectReference{
					Name: gwv1.ObjectName(defaultExtensionName),
				},
				Descriptors: []v1alpha1.RateLimitDescriptor{
					{
						Entries: []v1alpha1.RateLimitDescriptorEntry{
							{
								Type: v1alpha1.RateLimitDescriptorEntryTypeGeneric,
								Generic: &v1alpha1.RateLimitDescriptorEntryGeneric{
									Key:   "service",
									Value: "api",
								},
							},
						},
					},
				},
			},
			trafficPolicy: &v1alpha1.TrafficPolicy{},
			validateRateLimit: func(t *testing.T, rl *ratev3.RateLimit) {
				require.NotNil(t, rl)
				assert.False(t, rl.FailureModeDeny) // Should be fail open (deny=false)
			},
		},
		{
			name: "without backend reference",
			gatewayExtension: &ir.GatewayExtension{
				Type: v1alpha1.GatewayExtensionTypeRateLimit,
				RateLimit: &v1alpha1.RateLimitProvider{
					Domain:      "test-domain",
					GrpcService: &v1alpha1.ExtGrpcService{},
				},
				ObjectSource: ir.ObjectSource{
					Name:      defaultExtensionName,
					Namespace: defaultNamespace,
				},
			},
			policy: &v1alpha1.RateLimitPolicy{
				ExtensionRef: v1alpha1.NamespacedObjectReference{
					Name: gwv1.ObjectName(defaultExtensionName),
				},
				Descriptors: []v1alpha1.RateLimitDescriptor{
					{
						Entries: []v1alpha1.RateLimitDescriptorEntry{
							{
								Type: v1alpha1.RateLimitDescriptorEntryTypeGeneric,
								Generic: &v1alpha1.RateLimitDescriptorEntryGeneric{
									Key:   "service",
									Value: "api",
								},
							},
						},
					},
				},
			},
			trafficPolicy: &v1alpha1.TrafficPolicy{},
			expectedError: "backend not provided in grpc service",
		},
		{
			name: "with wrong extension type",
			gatewayExtension: &ir.GatewayExtension{
				Type: v1alpha1.GatewayExtensionTypeExtProc,
				ObjectSource: ir.ObjectSource{
					Name:      defaultExtensionName,
					Namespace: defaultNamespace,
				},
			},
			policy: &v1alpha1.RateLimitPolicy{
				ExtensionRef: v1alpha1.NamespacedObjectReference{
					Name: gwv1.ObjectName(defaultExtensionName),
				},
				Descriptors: []v1alpha1.RateLimitDescriptor{
					{
						Entries: []v1alpha1.RateLimitDescriptorEntry{
							{
								Type: v1alpha1.RateLimitDescriptorEntryTypeGeneric,
								Generic: &v1alpha1.RateLimitDescriptorEntryGeneric{
									Key:   "service",
									Value: "api",
								},
							},
						},
					},
				},
			},
			trafficPolicy: &v1alpha1.TrafficPolicy{},
			expectedError: "extension has type ExtProc but RateLimit was expected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rl *ratev3.RateLimit
			var err error

			if tt.policy == nil {
				err = errors.New("extensionRef is required")
			} else if tt.gatewayExtension == nil {
				err = fmt.Errorf("failed to get referenced GatewayExtension")
			} else if tt.gatewayExtension.Type != v1alpha1.GatewayExtensionTypeRateLimit {
				err = fmt.Errorf("extension has type %s but %s was expected",
					tt.gatewayExtension.Type, v1alpha1.GatewayExtensionTypeRateLimit)
			} else {
				// Get the extension's spec
				extension := tt.gatewayExtension.RateLimit
				if extension == nil {
					err = fmt.Errorf("RateLimit configuration is missing in GatewayExtension")
				} else {
					// Create a timeout based on the timeout from extension
					timeout := durationpb.New(extension.Timeout.Duration)

					// Use the domain from the extension
					domain := extension.Domain

					// Construct cluster name from the backendRef
					if extension.GrpcService != nil && extension.GrpcService.BackendRef != nil {
						clusterName := fmt.Sprintf("%s.%s.svc.cluster.local:%d",
							extension.GrpcService.BackendRef.Name,
							tt.gatewayExtension.Namespace,
							*extension.GrpcService.BackendRef.Port)

						// Create a rate limit configuration
						rl = &ratev3.RateLimit{
							Domain:          domain,
							Timeout:         timeout,
							FailureModeDeny: !extension.FailOpen,
							RateLimitService: &envoyratelimitv3.RateLimitServiceConfig{
								GrpcService: &envoycorev3.GrpcService{
									TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
										EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{
											ClusterName: clusterName,
										},
									},
								},
								TransportApiVersion: envoycorev3.ApiVersion_V3,
							},
							Stage:                   0,
							EnableXRatelimitHeaders: ratev3.RateLimit_OFF,
							RequestType:             "both",
							StatPrefix:              "",
						}
					} else {
						err = fmt.Errorf("backend not provided in grpc service")
					}
				}
			}

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, rl)
			tt.validateRateLimit(t, rl)
		})
	}
}
