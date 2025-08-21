package validator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name         string
		options      []Option
		setupEnvoy   bool
		expectedType string
		expectedPath string
		expectedImg  string
	}{
		{
			name:         "default - returns binary validator when envoy executable exists",
			options:      nil,
			setupEnvoy:   true,
			expectedType: "*validator.binaryValidator",
		},
		{
			name:         "default - returns docker validator when envoy not in path",
			options:      nil,
			setupEnvoy:   false,
			expectedType: "*validator.dockerValidator",
		},
		{
			name:         "custom binary path - overrides binary validator when exists",
			options:      nil,
			setupEnvoy:   true,
			expectedType: "*validator.binaryValidator",
		},
		{
			name: "custom docker image - override default envoy image",
			options: []Option{
				WithDockerImage("envoyproxy/envoy:v1.28.0"),
			},
			setupEnvoy:   false,
			expectedType: "*validator.dockerValidator",
			expectedImg:  "envoyproxy/envoy:v1.28.0",
		},
		{
			name: "custom both - binary takes precedence when exists",
			options: []Option{
				WithBinaryPath("/path/to/envoy"),
				WithDockerImage("custom/envoy:latest"),
			},
			setupEnvoy:   true,
			expectedType: "*validator.binaryValidator",
		},
		{
			name: "custom both - docker fallback when binary not found and docker image is set",
			options: []Option{
				WithBinaryPath("/nonexistent/envoy"),
				WithDockerImage("custom/envoy:latest"),
			},
			setupEnvoy:   false,
			expectedType: "*validator.dockerValidator",
			expectedImg:  "custom/envoy:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tmpFile *os.File
			if tt.setupEnvoy {
				var err error
				tmpFile, err = os.CreateTemp("", "envoy")
				require.NoError(t, err)
				defer os.Remove(tmpFile.Name())
				require.NoError(t, os.Chmod(tmpFile.Name(), 0755))

				// Set up custom binary path options for relevant tests
				if tt.name == "custom binary path - binary validator" {
					tt.options = []Option{WithBinaryPath(tmpFile.Name())}
					tt.expectedPath = tmpFile.Name()
				} else if tt.name == "custom both - binary takes precedence when exists" {
					tt.options = []Option{
						WithBinaryPath(tmpFile.Name()),
						WithDockerImage("custom/envoy:latest"),
					}
					tt.expectedPath = tmpFile.Name()
				} else if tt.options == nil {
					// For default tests, modify global defaultEnvoyPath temporarily
					origEnvoyPath := defaultEnvoyPath
					defaultEnvoyPath = tmpFile.Name()
					defer func() { defaultEnvoyPath = origEnvoyPath }()
				}
			}

			validator := New(tt.options...)
			assert.Equal(t, tt.expectedType, fmt.Sprintf("%T", validator))

			// Verify the internal configuration
			switch v := validator.(type) {
			case *binaryValidator:
				if tt.expectedPath != "" {
					assert.Equal(t, tt.expectedPath, v.path)
				}
			case *dockerValidator:
				if tt.expectedImg != "" {
					assert.Equal(t, tt.expectedImg, v.img)
				}
			}
		})
	}
}

func TestBinaryValidator_Validate(t *testing.T) {
	// note: actual config content doesn't matter for these tests. we cannot easily
	// test valid/invalid config with the binary validator, so we mock it as there's no
	// guarantee that the envoy binary is available and we cannot force it to be
	// due to multi-arch issues. instead, invalid configuration is tested in the docker
	// validator tests.
	tests := []struct {
		name        string
		yaml        string
		mockBinary  func(t *testing.T) string
		expectError bool
		errorMsg    string
	}{
		{
			name: "successful validation",
			yaml: "any-config-here",
			mockBinary: func(t *testing.T) string {
				script := `#!/bin/sh
if [ "$1" != "--mode" ] || [ "$2" != "validate" ] || [ "$3" != "--config-yaml" ]; then
    echo "Invalid arguments, expected: --mode validate --config-yaml" >&2
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
			yaml: "any-config-here", // actual config content doesn't matter for this test
			mockBinary: func(t *testing.T) string {
				script := `#!/bin/sh
if [ "$1" != "--mode" ] || [ "$2" != "validate" ] || [ "$3" != "--config-yaml" ]; then
    echo "Invalid arguments, expected: --mode validate --config-yaml" >&2
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
			yaml: "any-config-here", // actual config content doesn't matter for this test
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

			validator := &binaryValidator{path: mockPath}
			err := validator.Validate(context.Background(), tt.yaml)

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
		yaml        string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid configuration",
			yaml: `node:
  id: test-id
  cluster: test-cluster
static_resources:
  listeners:
    - name: listener_0
      address:
        socket_address:
          address: 0.0.0.0
          port_value: 10000
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: ingress_http
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: local_service
                      domains: ["*"]
                      routes:
                        - match:
                            prefix: "/"
                          route:
                            cluster: service_foo
  clusters:
    - name: service_foo
      connect_timeout: 0.25s
      type: STATIC
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: service_foo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address:
                      address: 127.0.0.1
                      port_value: 8080`,
			expectError: false,
		},
		{
			name: "missing listener address",
			yaml: `node:
  id: test-id
  cluster: test-cluster
static_resources:
  listeners:
    - name: listener_0
      # Missing required address field`,
			expectError: true,
			errorMsg:    `error initializing configuration '': error adding listener named 'listener_0': address is necessary`,
		},
		{
			name: "invalid regex in route match",
			yaml: `node:
  id: test-id
  cluster: test-cluster
static_resources:
  listeners:
    - name: listener_0
      address:
        socket_address:
          address: 0.0.0.0
          port_value: 10000
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: ingress_http
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: local_service
                      domains: ["*"]
                      routes:
                        - match:
                            safe_regex:
                              regex: "[[invalid.regex"  # Invalid regex pattern
                          route:
                            cluster: service_foo
  clusters:
    - name: service_foo
      connect_timeout: 0.25s
      type: STATIC
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: service_foo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address:
                      address: 127.0.0.1
                      port_value: 8080`,
			expectError: true,
			errorMsg:    `error initializing configuration '': missing ]:`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &dockerValidator{img: defaultEnvoyImage}
			err := validator.Validate(context.Background(), tt.yaml)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestStripDockerWarn(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no warning",
			input:    "normal error message",
			expected: "normal error message",
		},
		{
			name: "with platform warning",
			input: `WARNING: The requested image's platform (linux/amd64) does not match the detected host platform
Error in configuration`,
			expected: "Error in configuration",
		},
		{
			name: "multiple lines with warning",
			input: `WARNING: The requested image's platform (linux/amd64) does not match the detected host platform
Error line 1
Error line 2`,
			expected: "Error line 1\nError line 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripDockerWarn(tt.input)
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
	err = os.WriteFile(mockPath, []byte(script), 0755)
	require.NoError(t, err)

	return mockPath
}
