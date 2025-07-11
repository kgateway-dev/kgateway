package settings_test

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/settings"
)

func TestSettings(t *testing.T) {
	testCases := []struct {
		// name of the test case
		name string

		// env vars that are set at the beginning of test (and removed after test)
		envVars map[string]string

		// if set, then these are the expected populated settings
		expectedSettings *settings.Settings

		// if set, then an error parsing the settings is expected to occur
		expectedErrorStr string
	}{
		{
			// TODO: this test case does not fail when a new field is added to Settings
			// but not updated here. should it?
			name:    "defaults to empty or default values",
			envVars: map[string]string{},
			expectedSettings: &settings.Settings{
				DnsLookupFamily:             settings.DnsLookupFamilyV4Preferred,
				EnableIstioIntegration:      false,
				EnableIstioAutoMtls:         false,
				ListenerBindIpv6:            true,
				IstioNamespace:              "istio-system",
				XdsServiceName:              wellknown.DefaultXdsService,
				XdsServicePort:              wellknown.DefaultXdsPort,
				UseRustFormations:           false,
				EnableInferExt:              false,
				InferExtAutoProvision:       false,
				DefaultImageRegistry:        "cr.kgateway.dev",
				DefaultImageTag:             "",
				DefaultImagePullPolicy:      "IfNotPresent",
				WaypointLocalBinding:        false,
				IngressUseWaypoints:         false,
				LogLevel:                    "info",
				DiscoveryNamespaceSelectors: "[]",
				EnableAgentGateway:          false,
				WeightedRoutePrecedence:     false,
				RouteReplacementMode:        settings.RouteReplacementStandard,
				EnableBuiltinDefaultMetrics: false,
				GlobalPolicyNamespace:       "",
			},
		},
		{
			name: "all values set",
			envVars: map[string]string{
				"KGW_DNS_LOOKUP_FAMILY":              string(settings.DnsLookupFamilyV4Only),
				"KGW_ENABLE_ISTIO_INTEGRATION":       "true",
				"KGW_ENABLE_ISTIO_AUTO_MTLS":         "true",
				"KGW_LISTENER_BIND_IPV6":             "false",
				"KGW_STS_CLUSTER_NAME":               "my-cluster",
				"KGW_STS_URI":                        "my.sts.uri",
				"KGW_XDS_SERVICE_HOST":               "my-xds-host",
				"KGW_XDS_SERVICE_NAME":               "custom-svc",
				"KGW_XDS_SERVICE_PORT":               "1234",
				"KGW_USE_RUST_FORMATIONS":            "true",
				"KGW_ENABLE_INFER_EXT":               "true",
				"KGW_INFER_EXT_AUTO_PROVISION":       "true",
				"KGW_DEFAULT_IMAGE_REGISTRY":         "my-registry",
				"KGW_DEFAULT_IMAGE_TAG":              "my-tag",
				"KGW_DEFAULT_IMAGE_PULL_POLICY":      "Always",
				"KGW_WAYPOINT_LOCAL_BINDING":         "true",
				"KGW_INGRESS_USE_WAYPOINTS":          "true",
				"KGW_LOG_LEVEL":                      "debug",
				"KGW_DISCOVERY_NAMESPACE_SELECTORS":  `[{"matchExpressions":[{"key":"kubernetes.io/metadata.name","operator":"In","values":["infra"]}]},{"matchLabels":{"app":"a"}}]`,
				"KGW_ENABLE_AGENT_GATEWAY":           "true",
				"KGW_WEIGHTED_ROUTE_PRECEDENCE":      "true",
				"KGW_ROUTE_REPLACEMENT_MODE":         string(settings.RouteReplacementStrict),
				"KGW_ENABLE_BUILTIN_DEFAULT_METRICS": "true",
				"KGW_GLOBAL_POLICY_NAMESPACE":        "foo",
			},
			expectedSettings: &settings.Settings{
				DnsLookupFamily:             settings.DnsLookupFamilyV4Only,
				ListenerBindIpv6:            false,
				EnableIstioIntegration:      true,
				EnableIstioAutoMtls:         true,
				IstioNamespace:              "istio-system",
				XdsServiceHost:              "my-xds-host",
				XdsServiceName:              "custom-svc",
				XdsServicePort:              1234,
				UseRustFormations:           true,
				EnableInferExt:              true,
				InferExtAutoProvision:       true,
				DefaultImageRegistry:        "my-registry",
				DefaultImageTag:             "my-tag",
				DefaultImagePullPolicy:      "Always",
				WaypointLocalBinding:        true,
				IngressUseWaypoints:         true,
				LogLevel:                    "debug",
				DiscoveryNamespaceSelectors: `[{"matchExpressions":[{"key":"kubernetes.io/metadata.name","operator":"In","values":["infra"]}]},{"matchLabels":{"app":"a"}}]`,
				EnableAgentGateway:          true,
				WeightedRoutePrecedence:     true,
				RouteReplacementMode:        settings.RouteReplacementStrict,
				EnableBuiltinDefaultMetrics: true,
				GlobalPolicyNamespace:       "foo",
			},
		},
		{
			name: "errors on invalid bool",
			envVars: map[string]string{
				"KGW_ENABLE_ISTIO_INTEGRATION": "true123",
			},
			expectedErrorStr: "invalid syntax",
		},
		{
			name: "errors on invalid port",
			envVars: map[string]string{
				"KGW_XDS_SERVICE_PORT": "a123",
			},
			expectedErrorStr: "invalid syntax",
		},
		{
			name: "errors on invalid dns lookup family",
			envVars: map[string]string{
				"KGW_DNS_LOOKUP_FAMILY": "invalid",
			},
			expectedErrorStr: `invalid DNS lookup family: "invalid"`,
		},
		{
			name: "errors on invalid route replacement mode",
			envVars: map[string]string{
				"KGW_ROUTE_REPLACEMENT_MODE": "invalid",
			},
			expectedErrorStr: `invalid route replacement mode: "invalid"`,
		},
		{
			name: "ignores other env vars",
			envVars: map[string]string{
				"KGW_DOES_NOT_EXIST":         "true",
				"ANOTHER_VAR":                "abc",
				"KGW_ENABLE_ISTIO_AUTO_MTLS": "true",
			},
			expectedSettings: &settings.Settings{
				DnsLookupFamily:             settings.DnsLookupFamilyV4Preferred,
				EnableIstioAutoMtls:         true,
				ListenerBindIpv6:            true,
				IstioNamespace:              "istio-system",
				XdsServiceName:              wellknown.DefaultXdsService,
				XdsServicePort:              wellknown.DefaultXdsPort,
				DefaultImageRegistry:        "cr.kgateway.dev",
				DefaultImageTag:             "",
				DefaultImagePullPolicy:      "IfNotPresent",
				WaypointLocalBinding:        false,
				IngressUseWaypoints:         false,
				LogLevel:                    "info",
				DiscoveryNamespaceSelectors: "[]",
				EnableAgentGateway:          false,
				WeightedRoutePrecedence:     false,
				RouteReplacementMode:        settings.RouteReplacementStandard,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				cleanupEnvVars(t, tc.envVars)
			})

			for k, v := range tc.envVars {
				if err := os.Setenv(k, v); err != nil {
					t.Fatalf("Failed to set environment variable %s=%s: %v", k, v, err)
				}
			}

			s, err := settings.BuildSettings()

			if tc.expectedErrorStr != "" {
				require.ErrorContains(t, err, tc.expectedErrorStr)
				return
			}

			require.NoError(t, err)

			diff := cmp.Diff(tc.expectedSettings, s)
			require.Emptyf(t, diff, "Settings do not match expected values (-expected +got):\n%s", diff)
		})
	}
}

func cleanupEnvVars(t *testing.T, envVars map[string]string) {
	t.Helper()
	for k := range envVars {
		if err := os.Unsetenv(k); err != nil {
			t.Errorf("Failed to unset environment variable %s: %v", k, err)
		}
	}
}
