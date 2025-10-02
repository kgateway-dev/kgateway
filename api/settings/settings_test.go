package settings

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

// allEnvVarsSet returns a map which contains keys corresponding to every ENV var that can be used to configure settings,
// with values set to a non-default value.
func allEnvVarsSet() map[string]string {
	return map[string]string{
		"KGW_DNS_LOOKUP_FAMILY":              string(DnsLookupFamilyV4Only),
		"KGW_LISTENER_BIND_IPV6":             "false",
		"KGW_ENABLE_ISTIO_INTEGRATION":       "true",
		"KGW_ENABLE_ISTIO_AUTO_MTLS":         "true",
		"KGW_ISTIO_NAMESPACE":                "my-istio-namespace",
		"KGW_XDS_SERVICE_HOST":               "my-xds-host",
		"KGW_XDS_SERVICE_NAME":               "custom-svc",
		"KGW_XDS_SERVICE_PORT":               "1234",
		"KGW_AGENTGATEWAY_XDS_SERVICE_PORT":  "5678",
		"KGW_USE_RUST_FORMATIONS":            "true",
		"KGW_ENABLE_INFER_EXT":               "true",
		"KGW_INFER_EXT_AUTO_PROVISION":       "true",
		"KGW_DEFAULT_IMAGE_REGISTRY":         "my-registry",
		"KGW_DEFAULT_IMAGE_TAG":              "my-tag",
		"KGW_DEFAULT_IMAGE_PULL_POLICY":      "Always",
		"KGW_WAYPOINT_LOCAL_BINDING":         "true",
		"KGW_INGRESS_USE_WAYPOINTS":          "false",
		"KGW_LOG_LEVEL":                      "debug",
		"KGW_DISCOVERY_NAMESPACE_SELECTORS":  `[{"matchExpressions":[{"key":"kubernetes.io/metadata.name","operator":"In","values":["infra"]}]},{"matchLabels":{"app":"a"}}]`,
		"KGW_ENABLE_AGENTGATEWAY":            "true",
		"KGW_WEIGHTED_ROUTE_PRECEDENCE":      "true",
		"KGW_VALIDATION_MODE":                string(ValidationStrict),
		"KGW_ENABLE_BUILTIN_DEFAULT_METRICS": "true",
		"KGW_GLOBAL_POLICY_NAMESPACE":        "foo",
		"KGW_DISABLE_LEADER_ELECTION":        "true",
		"KGW_POLICY_MERGE":                   `{"TrafficPolicy":{"extProc":"DeepMerge"}}`,
		"KGW_ENABLE_WAYPOINT":                "true",
		"KGW_XDS_AUTH":                       "false",
	}
}

func TestSettings(t *testing.T) {
	testCases := []struct {
		// name of the test case
		name string

		// env vars that are set at the beginning of test (and removed after test)
		envVars map[string]string

		// if set, then these are the expected populated settings
		expectedSettings *Settings

		// if set, then an error parsing the settings is expected to occur
		expectedErrorStr string
	}{
		{
			// This test will pass if a new field is added to Settings and the default value is null value for the type.
			// In this case the test will still be testing that expected default values are set, though our convention is to set it explicitly.
			name:    "defaults to empty or default values",
			envVars: map[string]string{},
			expectedSettings: &Settings{
				DnsLookupFamily:             DnsLookupFamilyV4Preferred,
				ListenerBindIpv6:            true,
				EnableIstioIntegration:      false,
				EnableIstioAutoMtls:         false,
				IstioNamespace:              "istio-system",
				XdsServiceHost:              "",
				XdsServiceName:              wellknown.DefaultXdsService,
				XdsServicePort:              wellknown.DefaultXdsPort,
				AgentgatewayXdsServicePort:  wellknown.DefaultAgwXdsPort,
				UseRustFormations:           false,
				EnableInferExt:              false,
				InferExtAutoProvision:       false,
				DefaultImageRegistry:        "cr.kgateway.dev",
				DefaultImageTag:             "",
				DefaultImagePullPolicy:      "IfNotPresent",
				WaypointLocalBinding:        false,
				IngressUseWaypoints:         true,
				LogLevel:                    "info",
				DiscoveryNamespaceSelectors: "[]",
				EnableAgentgateway:          false,
				WeightedRoutePrecedence:     false,
				ValidationMode:              ValidationStandard,
				EnableBuiltinDefaultMetrics: false,
				GlobalPolicyNamespace:       "",
				DisableLeaderElection:       false,
				PolicyMerge:                 "{}",
				EnableWaypoint:              false,
				XdsAuth:                     true,
			},
		},
		{
			// This test will pass if a new field is added to Settings and the default value is null value for the type.
			// However, a separate test will fail if a new field with a non-default value is not added to the map returned by allEnvVarsSet()
			name:    "all values set",
			envVars: allEnvVarsSet(),
			expectedSettings: &Settings{
				DnsLookupFamily:             DnsLookupFamilyV4Only,
				ListenerBindIpv6:            false,
				EnableIstioIntegration:      true,
				EnableIstioAutoMtls:         true,
				IstioNamespace:              "my-istio-namespace",
				XdsServiceHost:              "my-xds-host",
				XdsServiceName:              "custom-svc",
				XdsServicePort:              1234,
				AgentgatewayXdsServicePort:  5678,
				UseRustFormations:           true,
				EnableInferExt:              true,
				InferExtAutoProvision:       true,
				DefaultImageRegistry:        "my-registry",
				DefaultImageTag:             "my-tag",
				DefaultImagePullPolicy:      "Always",
				WaypointLocalBinding:        true,
				IngressUseWaypoints:         false,
				LogLevel:                    "debug",
				DiscoveryNamespaceSelectors: `[{"matchExpressions":[{"key":"kubernetes.io/metadata.name","operator":"In","values":["infra"]}]},{"matchLabels":{"app":"a"}}]`,
				EnableAgentgateway:          true,
				WeightedRoutePrecedence:     true,
				ValidationMode:              ValidationStrict,
				EnableBuiltinDefaultMetrics: true,
				GlobalPolicyNamespace:       "foo",
				DisableLeaderElection:       true,
				PolicyMerge:                 `{"TrafficPolicy":{"extProc":"DeepMerge"}}`,
				EnableWaypoint:              true,
				XdsAuth:                     false,
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
			name: "errors on invalid port",
			envVars: map[string]string{
				"KGW_AGENTGATEWAY_XDS_SERVICE_PORT": "a123",
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
			name: "errors on invalid validation mode",
			envVars: map[string]string{
				"KGW_VALIDATION_MODE": "invalid",
			},
			expectedErrorStr: `invalid validation mode: "invalid"`,
		},
		{
			name: "ignores other env vars",
			envVars: map[string]string{
				"KGW_DOES_NOT_EXIST":         "true",
				"ANOTHER_VAR":                "abc",
				"KGW_ENABLE_ISTIO_AUTO_MTLS": "true",
			},
			expectedSettings: &Settings{
				DnsLookupFamily:             DnsLookupFamilyV4Preferred,
				EnableIstioAutoMtls:         true,
				ListenerBindIpv6:            true,
				IstioNamespace:              "istio-system",
				XdsServiceName:              wellknown.DefaultXdsService,
				XdsServicePort:              wellknown.DefaultXdsPort,
				AgentgatewayXdsServicePort:  wellknown.DefaultAgwXdsPort,
				DefaultImageRegistry:        "cr.kgateway.dev",
				DefaultImageTag:             "",
				DefaultImagePullPolicy:      "IfNotPresent",
				WaypointLocalBinding:        false,
				IngressUseWaypoints:         true,
				LogLevel:                    "info",
				DiscoveryNamespaceSelectors: "[]",
				EnableAgentgateway:          false,
				WeightedRoutePrecedence:     false,
				ValidationMode:              ValidationStandard,
				PolicyMerge:                 "{}",
				XdsAuth:                     true,
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

			s, err := BuildSettings()

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

// TestEnvVarCoverage tests that all settings are tested with non-default values.
func TestEnvVarCoverage(t *testing.T) {
	s := Settings{}
	settingsValue := reflect.ValueOf(s)

	allEnvVars := allEnvVarsSet()
	// Check for the right number of env vars defined
	require.Equal(t, settingsValue.NumField(), len(allEnvVars), "Number of fields in Settings does not match number of tested env vars")

	// Check that each field of Settings has a corresponding env var set in the test map
	// This protects against typos when adding new settings to the test map.
	for envVar, defaultValue := range expectedEnvVars(settingsValue) {
		require.Contains(t, allEnvVars, envVar, "Env var %s is not tested", envVar)
		require.NotEqual(t, allEnvVars[envVar], defaultValue, "Env var %s is set to the default value", envVar)
	}
}

// TestExpectedEnvVars tests that the expectedEnvVars function, which is used only in this test file, parses the field tags correctly to calculate the env var names.
func TestExpectedEnvVars(t *testing.T) {
	validateValue := reflect.ValueOf(validateExpectedEnvs{})

	expectedEnvVars := expectedEnvVars(validateValue)
	require.Equal(t, len(expectedEnvVars), validateValue.NumField())

	// Check that the env vars are correct
	require.Contains(t, expectedEnvVars, "KGW_FIELD_ONE", "Env var KGW_FIELD_ONE is not set")
	require.Equal(t, expectedEnvVars["KGW_FIELD_ONE"], "default_value_1", "Env var KGW_FIELD_ONE is not set to the default value")
	require.Contains(t, expectedEnvVars, "KGW_FIELDTWO", "Env var KGW_FIELDTWO is not set")
	require.Equal(t, expectedEnvVars["KGW_FIELDTWO"], "", "Env var KGW_FIELDTWO is not set to the default value")
	require.Contains(t, expectedEnvVars, "ALT_FIELD_3", "Env var ALT_FIELD_3 is not set")
	require.Contains(t, expectedEnvVars, "ALT_FIELD_4", "Env var ALT_FIELD_4 is not set")
	require.Contains(t, expectedEnvVars, "KGW_FIELD_SSL_CONFIG", "Env var KGW_FIELD_SSL_CONFIG is not set")
	require.Contains(t, expectedEnvVars, "KGW_FIELDHTTPCONFIG", "Env var KGW_FIELDHTTPCONFIG is not set")
}

func cleanupEnvVars(t *testing.T, envVars map[string]string) {
	t.Helper()
	for k := range envVars {
		if err := os.Unsetenv(k); err != nil {
			t.Errorf("Failed to unset environment variable %s: %v", k, err)
		}
	}
}

var (
	gatherRegexp  = regexp.MustCompile("([^A-Z]+|[A-Z]+[^A-Z]+|[A-Z]+)")
	acronymRegexp = regexp.MustCompile("([A-Z]+)([A-Z][^A-Z]+)")
)

// expectedEnvVars returns a map of all the env vars that should be set for the given Settings value.
// The value of the map is the default value of the field.
func expectedEnvVars(settingsValue reflect.Value) map[string]interface{} {
	// This is a modified version of the code in https://github.com/kelseyhightower/envconfig/blob/7834011875d613aec60c606b52c2b0fe8949fe91/envconfig.go#L102-L128
	expectedEnvVars := make(map[string]interface{}, settingsValue.NumField())
	for i := 0; i < settingsValue.NumField(); i++ {
		fieldType := settingsValue.Type().Field(i)
		splitWords := fieldType.Tag.Get("split_words") == "true"

		envVarName := fieldType.Name
		if splitWords {
			words := gatherRegexp.FindAllStringSubmatch(fieldType.Name, -1)
			if len(words) > 0 {
				var name []string
				for _, words := range words {
					if m := acronymRegexp.FindStringSubmatch(words[0]); len(m) == 3 {
						name = append(name, m[1], m[2])
					} else {
						name = append(name, words[0])
					}
				}
				envVarName = strings.Join(name, "_")
			}
		}

		envVarName = strings.ToUpper(envVarName)
		// Always have a prefix
		envVarName = fmt.Sprintf("KGW_%s", envVarName)

		// If the field has an alt tag, use that as the env var name
		if fieldType.Tag.Get("alt") != "" {
			envVarName = fieldType.Tag.Get("alt")
		}
		expectedEnvVars[envVarName] = fieldType.Tag.Get("default")
	}
	return expectedEnvVars
}

// validateExpectedEnvs is used to validate the expectedEnvVars function.
type validateExpectedEnvs struct {
	// Field with split_words:true
	FieldOne string `split_words:"true" default:"default_value_1"`
	// Field with split_words:false
	FieldTwo string `split_words:"false"`
	// Field with split_words:true and alt name
	FieldThree string `split_words:"true" alt:"ALT_FIELD_3"`
	// Field with split_words:false and alt name
	FieldFour string `split_words:"false" alt:"ALT_FIELD_4"`
	// Field with acronym and split_words:true
	FieldSSLConfig string `split_words:"true"`
	// Field with acronym and split_words:false
	FieldHTTPConfig string `split_words:"false"`
}
