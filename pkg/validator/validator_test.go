package validator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhttpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoymatcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
)

func TestBinaryValidator_Validate(t *testing.T) {
	// note: actual config content doesn't matter for these tests. we cannot easily
	// test valid/invalid config with the binary validator, so we mock it as there's no
	// guarantee that the envoy binary is available and we cannot force it to be
	// due to multi-arch issues. instead, invalid configuration is tested in the docker
	// validator tests.
	tests := []struct {
		name        string
		bootstrap   *envoybootstrapv3.Bootstrap
		mockBinary  func(t *testing.T) string
		expectError bool
		errorMsg    string
	}{
		{
			name:      "successful validation",
			bootstrap: &envoybootstrapv3.Bootstrap{}, // actual config content doesn't matter for this test
			mockBinary: func(t *testing.T) string {
				script := `#!/bin/sh
if [ "$1" != "--mode" ] || [ "$2" != "validate" ] || [ "$3" != "--config-path" ]; then
    echo "Invalid arguments, expected: --mode validate --config-path" >&2
    exit 1
fi
exit 0
`
				return createMockBinary(t, script)
			},
			expectError: false,
		},
		{
			name:      "validation error with envoy-style message",
			bootstrap: &envoybootstrapv3.Bootstrap{}, // actual config content doesn't matter for this test
			mockBinary: func(t *testing.T) string {
				script := `#!/bin/sh
if [ "$1" != "--mode" ] || [ "$2" != "validate" ] || [ "$3" != "--config-path" ]; then
    echo "Invalid arguments, expected: --mode validate --config-path" >&2
    exit 1
fi
echo "error initializing configuration '': missing ]:" >&2
exit 1
`
				return createMockBinary(t, script)
			},
			expectError: true,
			errorMsg:    "invalid xds configuration: error initializing configuration '': missing ]:",
		},
		{
			name:      "binary execution failure",
			bootstrap: &envoybootstrapv3.Bootstrap{}, // actual config content doesn't matter for this test
			mockBinary: func(t *testing.T) string {
				script := `#!/bin/sh
# Simulate a binary execution failure (e.g. segfault)
exit 2
`
				return createMockBinary(t, script)
			},
			expectError: true,
			errorMsg:    "invalid xds configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPath := tt.mockBinary(t)
			defer os.Remove(mockPath)

			validator := NewBinary(mockPath)
			err := validator.Validate(context.Background(), tt.bootstrap)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDockerValidator_Validate(t *testing.T) {
	tests := []struct {
		name        string
		bootstrap   *envoybootstrapv3.Bootstrap
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid configuration",
			bootstrap: &envoybootstrapv3.Bootstrap{
				Node: &envoycorev3.Node{
					Id:      "test-id",
					Cluster: "test-cluster",
				},
				StaticResources: &envoybootstrapv3.Bootstrap_StaticResources{
					Listeners: []*envoylistenerv3.Listener{
						{
							Name: "listener_0",
							Address: &envoycorev3.Address{
								Address: &envoycorev3.Address_SocketAddress{
									SocketAddress: &envoycorev3.SocketAddress{
										Address: "0.0.0.0",
										PortSpecifier: &envoycorev3.SocketAddress_PortValue{
											PortValue: 10000,
										},
									},
								},
							},
							FilterChains: []*envoylistenerv3.FilterChain{
								{
									Filters: []*envoylistenerv3.Filter{
										{
											Name: "envoy.filters.network.http_connection_manager",
											ConfigType: &envoylistenerv3.Filter_TypedConfig{
												TypedConfig: utils.MustMessageToAny(&envoy_hcm.HttpConnectionManager{
													StatPrefix: "ingress_http",
													RouteSpecifier: &envoy_hcm.HttpConnectionManager_RouteConfig{
														RouteConfig: &envoyroutev3.RouteConfiguration{
															Name: "local_route",
															VirtualHosts: []*envoyroutev3.VirtualHost{
																{
																	Name:    "local_service",
																	Domains: []string{"*"},
																	Routes: []*envoyroutev3.Route{
																		{
																			Match: &envoyroutev3.RouteMatch{
																				PathSpecifier: &envoyroutev3.RouteMatch_Prefix{
																					Prefix: "/",
																				},
																			},
																			Action: &envoyroutev3.Route_Route{
																				Route: &envoyroutev3.RouteAction{
																					ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{
																						Cluster: "service_foo",
																					},
																				},
																			},
																		},
																	},
																},
															},
														},
													},
													HttpFilters: []*envoy_hcm.HttpFilter{
														{
															Name: "envoy.filters.http.router",
															ConfigType: &envoy_hcm.HttpFilter_TypedConfig{
																TypedConfig: utils.MustMessageToAny(&envoyhttpv3.Router{}),
															},
														},
													},
												}),
											},
										},
									},
								},
							},
						},
					},
					Clusters: []*envoyclusterv3.Cluster{
						{
							Name: "service_foo",
							ConnectTimeout: &duration.Duration{
								Seconds: 10,
							},
							ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{
								Type: envoyclusterv3.Cluster_STATIC,
							},
							LbPolicy: envoyclusterv3.Cluster_ROUND_ROBIN,
							LoadAssignment: &envoyendpointv3.ClusterLoadAssignment{
								ClusterName: "service_foo",
								Endpoints: []*envoyendpointv3.LocalityLbEndpoints{
									{
										LbEndpoints: []*envoyendpointv3.LbEndpoint{
											{
												HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
													Endpoint: &envoyendpointv3.Endpoint{
														Address: &envoycorev3.Address{
															Address: &envoycorev3.Address_SocketAddress{
																SocketAddress: &envoycorev3.SocketAddress{
																	Address: "127.0.0.1",
																	PortSpecifier: &envoycorev3.SocketAddress_PortValue{
																		PortValue: 8080,
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "missing listener address",
			bootstrap: &envoybootstrapv3.Bootstrap{
				Node: &envoycorev3.Node{
					Id:      "test-id",
					Cluster: "test-cluster",
				},
				StaticResources: &envoybootstrapv3.Bootstrap_StaticResources{
					Listeners: []*envoylistenerv3.Listener{
						{
							Name: "listener_0",
						},
					},
				},
			},
			expectError: true,
			errorMsg:    `error initializing configuration '/dev/fd/0': error adding listener named 'listener_0': address is necessary`,
		},
		{
			name: "invalid regex in route match",
			bootstrap: &envoybootstrapv3.Bootstrap{
				Node: &envoycorev3.Node{
					Id:      "test-id",
					Cluster: "test-cluster",
				},
				StaticResources: &envoybootstrapv3.Bootstrap_StaticResources{
					Listeners: []*envoylistenerv3.Listener{
						{
							Name: "listener_0",
							Address: &envoycorev3.Address{
								Address: &envoycorev3.Address_SocketAddress{
									SocketAddress: &envoycorev3.SocketAddress{
										Address: "0.0.0.0",
										PortSpecifier: &envoycorev3.SocketAddress_PortValue{
											PortValue: 10000,
										},
									},
								},
							},
							FilterChains: []*envoylistenerv3.FilterChain{
								{
									Filters: []*envoylistenerv3.Filter{
										{
											Name: "envoy.filters.network.http_connection_manager",
											ConfigType: &envoylistenerv3.Filter_TypedConfig{
												TypedConfig: utils.MustMessageToAny(&envoy_hcm.HttpConnectionManager{
													StatPrefix: "ingress_http",
													RouteSpecifier: &envoy_hcm.HttpConnectionManager_RouteConfig{
														RouteConfig: &envoyroutev3.RouteConfiguration{
															Name: "local_route",
															VirtualHosts: []*envoyroutev3.VirtualHost{
																{
																	Name:    "local_service",
																	Domains: []string{"*"},
																	Routes: []*envoyroutev3.Route{
																		{
																			Match: &envoyroutev3.RouteMatch{
																				PathSpecifier: &envoyroutev3.RouteMatch_SafeRegex{
																					SafeRegex: &envoymatcherv3.RegexMatcher{
																						Regex: "[[invalid.regex",
																					},
																				},
																			},
																			Action: &envoyroutev3.Route_Route{
																				Route: &envoyroutev3.RouteAction{
																					ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{
																						Cluster: "service_foo",
																					},
																				},
																			},
																		},
																	},
																},
															},
														},
													},
													HttpFilters: []*envoy_hcm.HttpFilter{
														{
															Name: "envoy.filters.http.router",
															ConfigType: &envoy_hcm.HttpFilter_TypedConfig{
																TypedConfig: utils.MustMessageToAny(&envoyhttpv3.Router{}),
															},
														},
													},
												}),
											},
										},
									},
								},
							},
						},
					},
					Clusters: []*envoyclusterv3.Cluster{
						{
							Name: "service_foo",
							ConnectTimeout: &duration.Duration{
								Seconds: 10,
							},
							ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{
								Type: envoyclusterv3.Cluster_STATIC,
							},
							LbPolicy: envoyclusterv3.Cluster_ROUND_ROBIN,
							LoadAssignment: &envoyendpointv3.ClusterLoadAssignment{
								ClusterName: "service_foo",
								Endpoints: []*envoyendpointv3.LocalityLbEndpoints{
									{
										LbEndpoints: []*envoyendpointv3.LbEndpoint{
											{
												HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
													Endpoint: &envoyendpointv3.Endpoint{
														Address: &envoycorev3.Address{
															Address: &envoycorev3.Address_SocketAddress{
																SocketAddress: &envoycorev3.SocketAddress{
																	Address: "127.0.0.1",
																	PortSpecifier: &envoycorev3.SocketAddress_PortValue{
																		PortValue: 8080,
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    `error initializing configuration '/dev/fd/0': missing ]:`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewDocker()
			err := validator.Validate(context.Background(), tt.bootstrap)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestExtractEnvoyError(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no error message",
			input:    "some random output\nno errors here",
			expected: "",
		},
		{
			name:     "simple error message",
			input:    "error initializing configuration '': invalid named capture group: (?<=foo)bar",
			expected: "error initializing configuration '': invalid named capture group: (?<=foo)bar",
		},
		{
			name: "error message with context",
			input: `error initializing configuration '/dev/fd/0': missing ]:
  in regex filter at line 42
  validation context: http_connection_manager`,
			expected: "error initializing configuration '/dev/fd/0': missing ]: in regex filter at line 42 validation context: http_connection_manager",
		},
		{
			name: "docker pull logs present",
			input: `Unable to find image 'quay.io/solo-io/envoy-gloo:1.36.3-patch1' locally
1.35.2-patch1: Pulling from solo-io/envoy-gloo
f90c8eb4724c: Pulling fs layer
9f37c34398c2: Pulling fs layer
1cc4dfe322cb: Pulling fs layer
e800bbdc2f77: Pulling fs layer
e800bbdc2f77: Waiting
1cc4dfe322cb: Download complete
9f37c34398c2: Verifying Checksum
9f37c34398c2: Download complete
f90c8eb4724c: Verifying Checksum
f90c8eb4724c: Download complete
e800bbdc2f77: Verifying Checksum
e800bbdc2f77: Download complete
f90c8eb4724c: Pull complete
9f37c34398c2: Pull complete
1cc4dfe322cb: Pull complete
e800bbdc2f77: Pull complete
Digest: sha256:98c645568997299a1c4301e6077a1d2f566bb20828c0739e6c4177a821524dad
Status: Downloaded newer image for quay.io/solo-io/envoy-gloo:1.36.3-patch1
error initializing configuration '/dev/fd/0': invalid named capture group: (?<=foo)bar`,
			expected: "error initializing configuration '/dev/fd/0': invalid named capture group: (?<=foo)bar",
		},
		{
			name: "docker pull logs with multi-line error",
			input: `Unable to find image 'quay.io/solo-io/envoy-gloo:1.36.3-patch1' locally
1.35.2-patch1: Pulling from solo-io/envoy-gloo
f90c8eb4724c: Pull complete
Status: Downloaded newer image for quay.io/solo-io/envoy-gloo:1.36.3-patch1
error initializing configuration '/dev/fd/0': missing ]:
  at line 42 in filter configuration
  regex validation failed`,
			expected: "error initializing configuration '/dev/fd/0': missing ]: at line 42 in filter configuration regex validation failed",
		},
		{
			name: "platform warning with error",
			input: `WARNING: The requested image's platform (linux/amd64) does not match the detected host platform
error initializing configuration '/dev/fd/0': listener validation failed
  invalid port configuration`,
			expected: "error initializing configuration '/dev/fd/0': listener validation failed invalid port configuration",
		},
		{
			name: "error with empty lines",
			input: `error initializing configuration '/dev/fd/0': validation error

additional context here

more details`,
			expected: "error initializing configuration '/dev/fd/0': validation error additional context here more details",
		},
		{
			name:     "strips goo.gle/debugonly prefix",
			input:    "error initializing configuration '/dev/fd/0': goo.gle/debugonly node { id: \"test\" } : Proto constraint validation failed",
			expected: "error initializing configuration '/dev/fd/0': node { id: \"test\" } : Proto constraint validation failed",
		},
		{
			name:     "strips goo.gle/debugproto prefix",
			input:    "error initializing configuration '/dev/fd/0': goo.gle/debugproto node { id: \"test\" } : Proto constraint validation failed",
			expected: "error initializing configuration '/dev/fd/0': node { id: \"test\" } : Proto constraint validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractEnvoyError(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func createMockBinary(t *testing.T, script string) string {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "mock-envoy")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	mockPath := filepath.Join(tmpDir, "mock-envoy")
	err = os.WriteFile(mockPath, []byte(script), 0o755) //nolint:gosec // G306: test file creating executable mock script
	require.NoError(t, err)

	return mockPath
}
