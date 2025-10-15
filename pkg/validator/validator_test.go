package validator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBinaryValidator_Validate(t *testing.T) {
	// note: actual config content doesn't matter for these tests. we cannot easily
	// test valid/invalid config with the binary validator, so we mock it as there's no
	// guarantee that the envoy binary is available and we cannot force it to be
	// due to multi-arch issues. instead, invalid configuration is tested in the docker
	// validator tests.
	tests := []struct {
		name        string
		json        string
		mockBinary  func(t *testing.T) string
		expectError bool
		errorMsg    string
	}{
		{
			name: "successful validation",
			json: "any-config-here",
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
			name: "validation error with envoy-style message",
			json: "any-config-here", // actual config content doesn't matter for this test
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
			name: "binary execution failure",
			json: "any-config-here", // actual config content doesn't matter for this test
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
			err := validator.Validate(context.Background(), tt.json)
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
		json        string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid configuration",
			json: `{
  "node": {
    "id": "test-id",
    "cluster": "test-cluster"
  },
  "static_resources": {
    "listeners": [
      {
        "name": "listener_0",
        "address": {
          "socket_address": {
            "address": "0.0.0.0",
            "port_value": 10000
          }
        },
        "filter_chains": [
          {
            "filters": [
              {
                "name": "envoy.filters.network.http_connection_manager",
                "typed_config": {
                  "@type": "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager",
                  "stat_prefix": "ingress_http",
                  "route_config": {
                    "name": "local_route",
                    "virtual_hosts": [
                      {
                        "name": "local_service",
                        "domains": ["*"],
                        "routes": [
                          {
                            "match": {
                              "prefix": "/"
                            },
                            "route": {
                              "cluster": "service_foo"
                            }
                          }
                        ]
                      }
                    ]
                  },
                  "http_filters": [
                    {
                      "name": "envoy.filters.http.router",
                      "typed_config": {
                        "@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"
                      }
                    }
                  ]
                }
              }
            ]
          }
        ]
      }
    ],
    "clusters": [
      {
        "name": "service_foo",
        "connect_timeout": "0.25s",
        "type": "STATIC",
        "lb_policy": "ROUND_ROBIN",
        "load_assignment": {
          "cluster_name": "service_foo",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "127.0.0.1",
                        "port_value": 8080
                      }
                    }
                  }
                }
              ]
            }
          ]
        }
      }
    ]
  }
}`,
			expectError: false,
		},
		{
			name: "missing listener address",
			json: `{
  "node": {
    "id": "test-id",
    "cluster": "test-cluster"
  },
  "static_resources": {
    "listeners": [
      {
        "name": "listener_0"
      }
    ]
  }
}`,
			expectError: true,
			errorMsg:    `error initializing configuration '/dev/fd/0': error adding listener named 'listener_0': address is necessary`,
		},
		{
			name: "invalid regex in route match",
			json: `{
  "node": {
    "id": "test-id",
    "cluster": "test-cluster"
  },
  "static_resources": {
    "listeners": [
      {
        "name": "listener_0",
        "address": {
          "socket_address": {
            "address": "0.0.0.0",
            "port_value": 10000
          }
        },
        "filter_chains": [
          {
            "filters": [
              {
                "name": "envoy.filters.network.http_connection_manager",
                "typed_config": {
                  "@type": "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager",
                  "stat_prefix": "ingress_http",
                  "route_config": {
                    "name": "local_route",
                    "virtual_hosts": [
                      {
                        "name": "local_service",
                        "domains": ["*"],
                        "routes": [
                          {
                            "match": {
                              "safe_regex": {
                                "regex": "[[invalid.regex"
                              }
                            },
                            "route": {
                              "cluster": "service_foo"
                            }
                          }
                        ]
                      }
                    ]
                  },
                  "http_filters": [
                    {
                      "name": "envoy.filters.http.router",
                      "typed_config": {
                        "@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"
                      }
                    }
                  ]
                }
              }
            ]
          }
        ]
      }
    ],
    "clusters": [
      {
        "name": "service_foo",
        "connect_timeout": "0.25s",
        "type": "STATIC",
        "lb_policy": "ROUND_ROBIN",
        "load_assignment": {
          "cluster_name": "service_foo",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "127.0.0.1",
                        "port_value": 8080
                      }
                    }
                  }
                }
              ]
            }
          ]
        }
      }
    ]
  }
}`,
			expectError: true,
			errorMsg:    `error initializing configuration '/dev/fd/0': missing ]:`,
		},
		{
			// should not error with argument too long
			// empty error msg due to too long since it tries to print entire invalid json
			name:        "validate very large config",
			json:        strings.Repeat("1", 1000000),
			expectError: true,
			errorMsg:    ``,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewDocker()
			err := validator.Validate(context.Background(), tt.json)

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
			input: `Unable to find image 'quay.io/solo-io/envoy-gloo:1.35.2-patch4' locally
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
Status: Downloaded newer image for quay.io/solo-io/envoy-gloo:1.35.2-patch4
error initializing configuration '/dev/fd/0': invalid named capture group: (?<=foo)bar`,
			expected: "error initializing configuration '/dev/fd/0': invalid named capture group: (?<=foo)bar",
		},
		{
			name: "docker pull logs with multi-line error",
			input: `Unable to find image 'quay.io/solo-io/envoy-gloo:1.35.2-patch4' locally
1.35.2-patch1: Pulling from solo-io/envoy-gloo
f90c8eb4724c: Pull complete
Status: Downloaded newer image for quay.io/solo-io/envoy-gloo:1.35.2-patch4
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
	err = os.WriteFile(mockPath, []byte(script), 0755) //nolint:gosec // G306: test file creating executable mock script
	require.NoError(t, err)

	return mockPath
}
