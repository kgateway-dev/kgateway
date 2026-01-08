package helm

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
)

func TestHelmChartVersionAndAppVersion(t *testing.T) {
	goldenFile := filepath.Join("testdata", "helm-version-output.golden")
	helmChartPath := filepath.Join("..", "..", "install", "helm", "kgateway")

	absGoldenFile, err := filepath.Abs(goldenFile)
	require.NoError(t, err, "failed to get absolute path for golden file")

	absHelmChartPath, err := filepath.Abs(helmChartPath)
	require.NoError(t, err, "failed to get absolute path for helm chart")

	_, err = os.Stat(absHelmChartPath)
	require.NoError(t, err, "helm chart not found at %s", absHelmChartPath)

	helmCmd := exec.Command("helm", "template", "foobar", absHelmChartPath, "--namespace", "default")
	grepCmd := exec.Command("grep", "-E", "-w", "-B", "1", "v0\\.0\\.[12]")

	helmOutput, err := helmCmd.StdoutPipe()
	require.NoError(t, err, "failed to create stdout pipe for helm command")

	grepCmd.Stdin = helmOutput
	var output bytes.Buffer
	grepCmd.Stdout = &output
	var stderr bytes.Buffer
	grepCmd.Stderr = &stderr

	err = helmCmd.Start()
	require.NoError(t, err, "failed to start helm command")

	err = grepCmd.Start()
	require.NoError(t, err, "failed to start grep command")

	err = helmCmd.Wait()
	require.NoError(t, err, "helm command failed")

	err = grepCmd.Wait()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// No matches found - output will be empty. Diff will fail; test will fail.
		} else {
			require.NoError(t, err, "grep command failed: %s", stderr.String())
		}
	}

	got := output.Bytes()

	refreshGolden := strings.ToLower(os.Getenv("REFRESH_GOLDEN"))
	if refreshGolden == "true" || refreshGolden == "1" {
		t.Log("REFRESH_GOLDEN is set, writing golden file", absGoldenFile)
		testdataDir := filepath.Dir(absGoldenFile)
		err = os.MkdirAll(testdataDir, 0o755)
		require.NoError(t, err, "failed to create testdata directory")

		err = os.WriteFile(absGoldenFile, got, 0o644) //nolint:gosec // G306: Golden test file can be readable
		require.NoError(t, err, "failed to write golden file")
		return
	}

	want, err := os.ReadFile(absGoldenFile)
	require.NoError(t, err, "failed to read golden file %s", absGoldenFile)

	diff := cmp.Diff(string(want), string(got))
	if diff != "" {
		t.Errorf("helm template output differs from golden file (-want +got):\n%s\n\nTo refresh: REFRESH_GOLDEN=true go test ./test/helm", diff)
	}
}

// TestHelmChartTemplate tests helm template output for both kgateway and agentgateway charts
// with different values configurations.
func TestHelmChartTemplate(t *testing.T) {
	charts := []string{"kgateway", "agentgateway"}

	valuesCases := []struct {
		name       string
		valuesYAML string
	}{
		{
			name:       "default",
			valuesYAML: "",
		},
		{
			name: "xds-tls-enabled",
			valuesYAML: `controller:
  xds:
    tls:
      enabled: true
`,
		},
		{
			name: "pdb-min-available",
			valuesYAML: `controller:
  podDisruptionBudget:
    minAvailable: 1
`,
		},
		{
			name: "pdb-max-unavailable",
			valuesYAML: `controller:
  podDisruptionBudget:
    maxUnavailable: 25%
`,
		},
	}

	for _, chart := range charts {
		for _, vc := range valuesCases {
			testName := chart + "/" + vc.name
			t.Run(testName, func(t *testing.T) {
				helmChartPath := filepath.Join("..", "..", "install", "helm", chart)
				absHelmChartPath, err := filepath.Abs(helmChartPath)
				require.NoError(t, err, "failed to get absolute path for helm chart")

				_, err = os.Stat(absHelmChartPath)
				require.NoError(t, err, "helm chart not found at %s", absHelmChartPath)

				// Build helm template command args
				// Explicitly set namespace to avoid picking up the current kubectl context's namespace
				args := []string{"template", "test-release", absHelmChartPath, "--namespace", "default"}

				// If we have custom values, write them to a temp file
				if vc.valuesYAML != "" {
					valuesFile, err := os.CreateTemp("", "values-*.yaml")
					require.NoError(t, err, "failed to create temp values file")
					defer os.Remove(valuesFile.Name())

					_, err = valuesFile.WriteString(vc.valuesYAML)
					require.NoError(t, err, "failed to write values file")
					err = valuesFile.Close()
					require.NoError(t, err, "failed to close values file")

					args = append(args, "-f", valuesFile.Name())
				}

				helmCmd := exec.Command("helm", args...)
				var output bytes.Buffer
				var stderr bytes.Buffer
				helmCmd.Stdout = &output
				helmCmd.Stderr = &stderr

				err = helmCmd.Run()
				require.NoError(t, err, "helm template failed: %s", stderr.String())

				got := output.Bytes()

				// Golden file path: testdata/<chart>/<values-case>.golden
				goldenDir := filepath.Join("testdata", chart)
				goldenFile := filepath.Join(goldenDir, vc.name+".golden")

				absGoldenFile, err := filepath.Abs(goldenFile)
				require.NoError(t, err, "failed to get absolute path for golden file")

				refreshGolden := strings.ToLower(os.Getenv("REFRESH_GOLDEN"))
				if refreshGolden == "true" || refreshGolden == "1" {
					t.Log("REFRESH_GOLDEN is set, writing golden file", absGoldenFile)
					err = os.MkdirAll(goldenDir, 0o755)
					require.NoError(t, err, "failed to create testdata directory")

					err = os.WriteFile(absGoldenFile, got, 0o644) //nolint:gosec // G306: Golden test file can be readable
					require.NoError(t, err, "failed to write golden file")
					return
				}

				want, err := os.ReadFile(absGoldenFile)
				require.NoError(t, err, "failed to read golden file %s; run with REFRESH_GOLDEN=true to generate", absGoldenFile)

				diff := cmp.Diff(string(want), string(got))
				if diff != "" {
					t.Errorf("helm template output differs from golden file (-want +got):\n%s\n\nTo refresh: REFRESH_GOLDEN=true go test ./test/helm", diff)
				}
			})
		}
	}
}
