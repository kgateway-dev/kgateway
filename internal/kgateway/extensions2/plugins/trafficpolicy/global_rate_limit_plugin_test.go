package trafficpolicy

import (
	"errors"
	"fmt"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v3"
	routeconfv3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	ratev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
	corev1 "k8s.io/api/core/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

func TestCreateRateLimitActions(t *testing.T) {
	tests := []struct {
		name           string
		descriptors    []v1alpha1.RateLimitDescriptor
		expectedError  string
		validateResult func(*testing.T, []*routeconfv3.RateLimit_Action)
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
			validateResult: func(t *testing.T, actions []*routeconfv3.RateLimit_Action) {
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
							Header: "X-User-ID",
						},
					},
				},
			},
			validateResult: func(t *testing.T, actions []*routeconfv3.RateLimit_Action) {
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
			validateResult: func(t *testing.T, actions []*routeconfv3.RateLimit_Action) {
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
			validateResult: func(t *testing.T, actions []*routeconfv3.RateLimit_Action) {
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
			validateResult: func(t *testing.T, actions []*routeconfv3.RateLimit_Action) {
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
							Header: "X-User-ID",
						},
					},
				},
			},
			validateResult: func(t *testing.T, actions []*routeconfv3.RateLimit_Action) {
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
				ExtensionRef: &corev1.LocalObjectReference{
					Name: defaultExtensionName,
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
				assert.Equal(t, corev3.ApiVersion_V3, rl.RateLimitService.TransportApiVersion)
				assert.Equal(t, ratev3.RateLimit_DRAFT_VERSION_03, rl.EnableXRatelimitHeaders)
				assert.Equal(t, "both", rl.RequestType)
				assert.Equal(t, rateLimitStatPrefix, rl.StatPrefix)
				assert.Equal(t, (*durationpb.Duration)(nil), rl.Timeout)
				assert.True(t, rl.FailureModeDeny) // Default should be failureModeAllow=false (deny)
			},
		},
		{
			name: "with custom timeout",
			gatewayExtension: &ir.GatewayExtension{
				Type: v1alpha1.GatewayExtensionTypeRateLimit,
				RateLimit: &v1alpha1.RateLimitProvider{
					Domain: "test-domain",
					GrpcService: &v1alpha1.ExtGrpcService{
						BackendRef: &gwv1alpha2.BackendRef{
							BackendObjectReference: createBackendRef(),
						},
					},
					Timeout: "5s",
				},
				ObjectSource: ir.ObjectSource{
					Name:      defaultExtensionName,
					Namespace: defaultNamespace,
				},
			},
			policy: &v1alpha1.RateLimitPolicy{
				ExtensionRef: &corev1.LocalObjectReference{
					Name: defaultExtensionName,
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
				ExtensionRef: &corev1.LocalObjectReference{
					Name: defaultExtensionName,
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
				assert.False(t, rl.FailureModeDeny) // Should be fail open (false)
			},
		},
		{
			name: "with invalid timeout",
			gatewayExtension: &ir.GatewayExtension{
				Type: v1alpha1.GatewayExtensionTypeRateLimit,
				RateLimit: &v1alpha1.RateLimitProvider{
					Domain: "test-domain",
					GrpcService: &v1alpha1.ExtGrpcService{
						BackendRef: &gwv1alpha2.BackendRef{
							BackendObjectReference: createBackendRef(),
						},
					},
					Timeout: "invalid",
				},
				ObjectSource: ir.ObjectSource{
					Name:      defaultExtensionName,
					Namespace: defaultNamespace,
				},
			},
			policy: &v1alpha1.RateLimitPolicy{
				ExtensionRef: &corev1.LocalObjectReference{
					Name: defaultExtensionName,
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
			expectedError: "invalid timeout in GatewayExtension test-ratelimit: time: invalid duration \"invalid\"",
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
				ExtensionRef: &corev1.LocalObjectReference{
					Name: defaultExtensionName,
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
				ExtensionRef: &corev1.LocalObjectReference{
					Name: defaultExtensionName,
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
		{
			name: "without extension reference",
			policy: &v1alpha1.RateLimitPolicy{
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
			expectedError: "extensionRef is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rl *ratev3.RateLimit
			var err error

			if tt.policy == nil || tt.policy.ExtensionRef == nil {
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
					// Create a timeout based on the time unit if specified, otherwise use the default
					var timeout *durationpb.Duration

					// Use timeout from extension if specified
					if extension.Timeout != "" {
						var parseDurationErr error
						duration, parseDurationErr := time.ParseDuration(string(extension.Timeout))
						if parseDurationErr != nil {
							err = fmt.Errorf("invalid timeout in GatewayExtension %s: %w",
								tt.gatewayExtension.Name, parseDurationErr)
						} else {
							timeout = durationpb.New(duration)
						}
					}

					if err == nil {
						// Use the domain from the extension
						domain := extension.Domain

						// Construct cluster name from the backendRef
						clusterName := ""
						if extension.GrpcService != nil && extension.GrpcService.BackendRef != nil {
							clusterName = fmt.Sprintf("%s.%s.svc.cluster.local:%d",
								extension.GrpcService.BackendRef.Name,
								tt.gatewayExtension.Namespace,
								*extension.GrpcService.BackendRef.Port)
						} else {
							err = fmt.Errorf("backend not provided in grpc service")
						}

						if err == nil {
							// Create a rate limit configuration
							rl = &ratev3.RateLimit{
								Domain:          domain,
								Timeout:         timeout,
								FailureModeDeny: !extension.FailOpen,
								RateLimitService: &ratelimitv3.RateLimitServiceConfig{
									GrpcService: &corev3.GrpcService{
										TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
											EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{
												ClusterName: clusterName,
											},
										},
									},
									TransportApiVersion: corev3.ApiVersion_V3,
								},
								Stage:                   0,
								EnableXRatelimitHeaders: ratev3.RateLimit_DRAFT_VERSION_03,
								RequestType:             "both",
								StatPrefix:              rateLimitStatPrefix,
							}
						}
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
