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

	helmCmd := exec.Command("helm", "template", "foobar", absHelmChartPath)
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
		t.Errorf("helm template output differs from golden file (-want +got):\n%s\n\nTo refresh: REFRESH_GOLDEN=true go test ./test/helm -run TestHelmTemplateVersion", diff)
	}
}
