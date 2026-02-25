package helm

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// helmValuesYAMLKeys returns the yaml tag names (before any comma) of all
func helmValuesYAMLKeys() []string {
	t := reflect.TypeOf(HelmValues{})
	keys := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		field := t.Field(i)
		tag := field.Tag.Get("yaml")
		name, _, _ := strings.Cut(tag, ",")
		if name != "" && name != "-" {
			keys = append(keys, name)
		}
	}
	sort.Strings(keys)
	return keys
}

// renderHelmValuesAsYAML marshals v into a temporary file and returns the path.
func renderHelmValuesAsYAML(t *testing.T, v HelmValues) string {
	t.Helper()
	data, err := yaml.Marshal(v)
	require.NoError(t, err, "failed to marshal HelmValues to YAML")

	valuesFile, err := os.CreateTemp("", "values-*.yaml")
	require.NoError(t, err, "failed to create temp values file")
	t.Cleanup(func() { os.Remove(valuesFile.Name()) })

	_, err = valuesFile.Write(data)
	require.NoError(t, err, "failed to write values file")
	require.NoError(t, valuesFile.Close(), "failed to close values file")

	return valuesFile.Name()
}

// runHelmTemplate runs `helm template test-release <chartPath> --namespace default`
func runHelmTemplate(t *testing.T, chartPath string, valuesFilePath string) string {
	t.Helper()
	absChartPath, err := filepath.Abs(chartPath)
	require.NoError(t, err, "failed to get absolute path for helm chart")

	args := []string{"template", "test-release", absChartPath, "--namespace", "default"}
	if valuesFilePath != "" {
		args = append(args, "-f", valuesFilePath)
	}

	helmCmd := exec.Command("helm", args...) 
	var stdout, stderr strings.Builder
	helmCmd.Stdout = &stdout
	helmCmd.Stderr = &stderr
	err = helmCmd.Run()
	require.NoError(t, err, "helm template failed: %s", stderr.String())
	return stdout.String()
}

// kgatewayChartPath is the chart path relative to this test file.
const kgatewayChartPath = "../../install/helm/kgateway"
// TestHelmValuesStructCoversYAMLKeys verifies that every top-level key present
// in values.yaml has a corresponding field (by yaml tag) in HelmValues, and
// that every HelmValues field also exists in values.yaml.
func TestHelmValuesStructCoversYAMLKeys(t *testing.T) {
	valuesFile := filepath.Join("..", "..", "install", "helm", "kgateway", "values.yaml")
	data, err := os.ReadFile(valuesFile)
	require.NoError(t, err, "failed to read values.yaml")

	var rawValues map[string]interface{}
	require.NoError(t, yaml.Unmarshal(data, &rawValues), "failed to unmarshal values.yaml")

	structKeys := helmValuesYAMLKeys()
	structKeySet := make(map[string]bool, len(structKeys))
	for _, k := range structKeys {
		structKeySet[k] = true
	}

	// Every values.yaml top-level key must be in the struct.
	for yamlKey := range rawValues {
		if !structKeySet[yamlKey] {
			t.Errorf("values.yaml key %q has no corresponding field in HelmValues; "+
				"add it to test/helm/values_types.go", yamlKey)
		}
	}

	// Every struct field must be in values.yaml.
	for _, structKey := range structKeys {
		if _, ok := rawValues[structKey]; !ok {
			t.Errorf("HelmValues field with yaml tag %q has no matching key in values.yaml; "+
				"either remove the field or add the key to values.yaml", structKey)
		}
	}
}

// TestHelmValuesXDSTLSEnabled verifies xDS TLS resources appear when enabled via the struct.
func TestHelmValuesXDSTLSEnabled(t *testing.T) {
	v := HelmValues{
		Controller: ControllerValues{
			XDS: XDSValues{
				TLS: XDSTLSValues{Enabled: true},
			},
		},
	}
	got := runHelmTemplate(t, kgatewayChartPath, renderHelmValuesAsYAML(t, v))

	if !strings.Contains(got, "kgateway-xds-cert") {
		t.Errorf("expected 'kgateway-xds-cert' in helm output when xds.tls.enabled=true")
	}
	if !strings.Contains(got, "KGW_XDS_TLS_ENABLED") {
		t.Errorf("expected 'KGW_XDS_TLS_ENABLED' env var when xds.tls.enabled=true")
	}
}

// TestHelmValuesXDSTLSDisabled verifies xDS TLS resources are absent by default.
func TestHelmValuesXDSTLSDisabled(t *testing.T) {
	got := runHelmTemplate(t, kgatewayChartPath, renderHelmValuesAsYAML(t, HelmValues{}))

	if strings.Contains(got, "kgateway-xds-cert") {
		t.Errorf("unexpected 'kgateway-xds-cert' in helm output when xds.tls.enabled=false")
	}
	if strings.Contains(got, "KGW_XDS_TLS_ENABLED") {
		t.Errorf("unexpected 'KGW_XDS_TLS_ENABLED' env var when xds.tls.enabled=false")
	}
}

// TestHelmValuesWaypointEnabled verifies that the waypoint env var is set when enabled.
func TestHelmValuesWaypointEnabled(t *testing.T) {
	v := HelmValues{
		Waypoint: WaypointValues{Enabled: true},
	}
	got := runHelmTemplate(t, kgatewayChartPath, renderHelmValuesAsYAML(t, v))

	if !strings.Contains(got, "KGW_ENABLE_WAYPOINT") {
		t.Errorf("expected 'KGW_ENABLE_WAYPOINT' env var when waypoint.enabled=true")
	}
}

// TestHelmValuesWaypointDisabled verifies that the waypoint env var is absent by default.
func TestHelmValuesWaypointDisabled(t *testing.T) {
	got := runHelmTemplate(t, kgatewayChartPath, renderHelmValuesAsYAML(t, HelmValues{}))

	if strings.Contains(got, "KGW_ENABLE_WAYPOINT") {
		t.Errorf("unexpected 'KGW_ENABLE_WAYPOINT' env var when waypoint.enabled=false")
	}
}

// TestHelmValuesValidationLevels verifies that each supported validation level
// is passed through to the controller env var correctly.
func TestHelmValuesValidationLevels(t *testing.T) {
	for _, level := range []string{"standard", "strict"} {
		t.Run(level, func(t *testing.T) {
			v := HelmValues{
				Validation: ValidationValues{Level: level},
			}
			got := runHelmTemplate(t, kgatewayChartPath, renderHelmValuesAsYAML(t, v))

			if !strings.Contains(got, level) {
				t.Errorf("expected validation level %q in helm output", level)
			}
		})
	}
}

// TestHelmValuesExtraEnv verifies that controller.extraEnv keys appear in the deployment.
func TestHelmValuesExtraEnv(t *testing.T) {
	v := HelmValues{
		Controller: ControllerValues{
			ExtraEnv: map[string]string{
				"MY_CUSTOM_VAR": "hello-world",
			},
		},
	}
	got := runHelmTemplate(t, kgatewayChartPath, renderHelmValuesAsYAML(t, v))

	if !strings.Contains(got, "MY_CUSTOM_VAR") {
		t.Errorf("expected 'MY_CUSTOM_VAR' in helm output when controller.extraEnv is set")
	}
}

// TestHelmValuesGatewayClassParametersRefs verifies that gatewayClassParametersRefs
// is serialised and passed to the KGW_GATEWAY_CLASS_PARAMETERS_REFS env var.
func TestHelmValuesGatewayClassParametersRefs(t *testing.T) {
	v := HelmValues{
		GatewayClassParametersRefs: map[string]GatewayClassParametersRef{
			"kgateway": {
				Name:      "shared-gwp",
				Namespace: "kgateway-system",
			},
		},
	}
	got := runHelmTemplate(t, kgatewayChartPath, renderHelmValuesAsYAML(t, v))

	if !strings.Contains(got, "KGW_GATEWAY_CLASS_PARAMETERS_REFS") {
		t.Errorf("expected 'KGW_GATEWAY_CLASS_PARAMETERS_REFS' env var in helm output")
	}
	if !strings.Contains(got, "shared-gwp") {
		t.Errorf("expected GatewayParameters name 'shared-gwp' in helm output")
	}
}

// TestHelmValuesDiscoveryNamespaceSelectors verifies that namespace selector
// entries are passed through to the KGW_DISCOVERY_NAMESPACE_SELECTORS env var.
func TestHelmValuesDiscoveryNamespaceSelectors(t *testing.T) {
	v := HelmValues{
		DiscoveryNamespaceSelectors: []map[string]interface{}{
			{
				"matchLabels": map[string]interface{}{
					"kubernetes.io/metadata.name": "my-namespace",
				},
			},
		},
	}
	got := runHelmTemplate(t, kgatewayChartPath, renderHelmValuesAsYAML(t, v))

	if !strings.Contains(got, "KGW_DISCOVERY_NAMESPACE_SELECTORS") {
		t.Errorf("expected 'KGW_DISCOVERY_NAMESPACE_SELECTORS' env var in helm output")
	}
	if !strings.Contains(got, "my-namespace") {
		t.Errorf("expected namespace selector value 'my-namespace' in helm output")
	}
}
func TestHelmValuesServiceAccountSuppressed(t *testing.T) {
	f := false
	v := HelmValues{
		ServiceAccount: ServiceAccountValues{
			Create: &f,
			Name:   "existing-sa",
		},
	}

	got := runHelmTemplate(t, kgatewayChartPath, renderHelmValuesAsYAML(t, v))

	if strings.Contains(got, "apiVersion: v1\nkind: ServiceAccount") {
		t.Errorf("unexpected ServiceAccount resource in helm output when serviceAccount.create=false")
	}
	if !strings.Contains(got, "serviceAccountName: existing-sa") {
		t.Errorf("expected deployment to reference 'existing-sa' when serviceAccount.create=false")
	}
}

func TestHelmValuesControllerImageOverride(t *testing.T) {
	v := HelmValues{
		Image: ImageValues{
			Tag: "v1.0.0",
		},
		Controller: ControllerValues{
			Image: ControllerImageValues{
				Registry:   "my.registry.io",
				Repository: "my-kgateway",
				Tag:        "v2.0.0",
			},
		},
	}
	got := runHelmTemplate(t, kgatewayChartPath, renderHelmValuesAsYAML(t, v))

	if !strings.Contains(got, "my.registry.io/my-kgateway:v2.0.0") {
		t.Errorf("expected controller pod image 'my.registry.io/my-kgateway:v2.0.0' in helm output")
	}
	if !strings.Contains(got, "value: v1.0.0") {
		t.Errorf("expected KGW_DEFAULT_IMAGE_TAG to be v1.0.0 (global image.tag) in helm output")
	}
}
func TestHelmValuesMarshalRoundTrip(t *testing.T) {
	original := HelmValues{
		CommonLabels: map[string]string{"env": "test"},
		Controller: ControllerValues{
			LogLevel: "debug",
			XDS:      XDSValues{TLS: XDSTLSValues{Enabled: true}},
			ExtraEnv: map[string]string{"FOO": "bar"},
		},
		Waypoint:   WaypointValues{Enabled: true},
		Validation: ValidationValues{Level: "strict"},
	}

	data, err := yaml.Marshal(original)
	require.NoError(t, err, "failed to marshal HelmValues")

	var restored HelmValues
	require.NoError(t, yaml.Unmarshal(data, &restored), "failed to unmarshal marshalled HelmValues")

	// Spot-check a few fields rather than a full reflect.DeepEqual to keep the
	// test readable and resilient to map ordering differences.
	if restored.Controller.LogLevel != original.Controller.LogLevel {
		t.Errorf("LogLevel round-trip: got %q, want %q", restored.Controller.LogLevel, original.Controller.LogLevel)
	}
	if restored.Controller.XDS.TLS.Enabled != original.Controller.XDS.TLS.Enabled {
		t.Errorf("XDS.TLS.Enabled round-trip: got %v, want %v", restored.Controller.XDS.TLS.Enabled, original.Controller.XDS.TLS.Enabled)
	}
	if restored.Waypoint.Enabled != original.Waypoint.Enabled {
		t.Errorf("Waypoint.Enabled round-trip: got %v, want %v", restored.Waypoint.Enabled, original.Waypoint.Enabled)
	}
	if restored.Validation.Level != original.Validation.Level {
		t.Errorf("Validation.Level round-trip: got %q, want %q", restored.Validation.Level, original.Validation.Level)
	}
	if restored.CommonLabels["env"] != original.CommonLabels["env"] {
		t.Errorf("CommonLabels round-trip: got %v, want %v", restored.CommonLabels, original.CommonLabels)
	}
}
