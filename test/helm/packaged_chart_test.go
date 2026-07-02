package helm

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
)

// TestPackagedChartRenderMatchesSource exercises the real packaging artifact
// produced by `make package-kgateway-charts` (the same target release.yaml
// runs). For each values permutation it renders both the chart source directory
// and the packaged .tgz, normalizes the fields the packaging step legitimately
// stamps (chart version, appVersion, and the image tag derived from
// appVersion), and asserts the renders are otherwise byte-identical.
//
// A divergence means packaging changed what ships to users -- e.g. a template
// excluded by .helmignore, or a file-layout change that drops a resource.
// Rendering every values case (not just the default) is deliberate: an optional
// template is only emitted under the values that enable it, so a dropped
// conditional template is caught only by the case that activates it.
//
// The packaged chart must already exist under _test/. When it is absent the
// test skips so that a plain `go test ./test/helm` stays fast and
// dependency-free; CI sets KGW_REQUIRE_PACKAGED_CHART=true (after running the
// package target) so a missing artifact fails loudly instead of silently
// skipping.
func TestPackagedChartRenderMatchesSource(t *testing.T) {
	const chartName = "kgateway"

	packagedChart, err := findPackagedChart(chartName)
	if err != nil {
		if envTruthy("KGW_REQUIRE_PACKAGED_CHART") {
			t.Fatalf("packaged chart required but not found: %v\nrun `make package-kgateway-charts` first", err)
		}
		t.Skipf("packaged chart not found, skipping parity check (run `make package-kgateway-charts` to enable): %v", err)
	}

	sourceChart, err := filepath.Abs(filepath.Join("..", "..", "install", "helm", chartName))
	require.NoError(t, err, "failed to resolve source chart path")

	for _, vc := range helmChartTemplateCases {
		t.Run(vc.name, func(t *testing.T) {
			source := normalizeChartVersionFields(string(renderChartAtPath(t, sourceChart, vc.valuesYAML, vc.apiVersions)))
			packaged := normalizeChartVersionFields(string(renderChartAtPath(t, packagedChart, vc.valuesYAML, vc.apiVersions)))

			if diff := cmp.Diff(source, packaged); diff != "" {
				t.Errorf("packaged chart render differs from source render after normalizing version fields (-source +packaged):\n%s\n\n"+
					"packaging changed what ships (e.g. a template dropped by .helmignore).\nsource:   %s\npackaged: %s",
					diff, sourceChart, packagedChart)
			}
		})
	}
}

// findPackagedChart returns the absolute path to the packaged chart tarball for
// chartName under _test/ (e.g. _test/kgateway-v1.0.1-dev.tgz). It excludes
// sibling charts that share the prefix, such as kgateway-crds-*.tgz.
func findPackagedChart(chartName string) (string, error) {
	testAssetDir := filepath.Join("..", "..", "_test")
	matches, err := filepath.Glob(filepath.Join(testAssetDir, chartName+"-*.tgz"))
	if err != nil {
		return "", err
	}

	var candidates []string
	for _, m := range matches {
		// Strip the chart name and an optional leading 'v'; a version always
		// starts with a digit, so "kgateway-v1.0.1.tgz" and "kgateway-1.0.1.tgz"
		// match while the "kgateway-crds-..." sibling does not.
		rest := strings.TrimPrefix(filepath.Base(m), chartName+"-")
		rest = strings.TrimPrefix(rest, "v")
		if rest != "" && rest[0] >= '0' && rest[0] <= '9' {
			candidates = append(candidates, m)
		}
	}

	switch len(candidates) {
	case 0:
		return "", fmt.Errorf("no %s-<version>.tgz found under %s", chartName, testAssetDir)
	case 1:
		return filepath.Abs(candidates[0])
	default:
		return "", fmt.Errorf("expected exactly one %s tarball under %s, found %d: %v", chartName, testAssetDir, len(candidates), candidates)
	}
}

// chartVersionFieldNormalizers replace the fields that `helm package` stamps
// from Chart.yaml's version/appVersion (which differ between the source chart
// and a release-versioned package) with a fixed placeholder, so renders can be
// compared for any other difference.
var chartVersionFieldNormalizers = []struct {
	re   *regexp.Regexp
	repl string
}{
	// helm.sh/chart: kgateway-<chart version>
	{regexp.MustCompile(`(?m)^(\s*helm\.sh/chart: kgateway-).*$`), `${1}NORMALIZED`},
	// app.kubernetes.io/version: "<appVersion>"
	{regexp.MustCompile(`(?m)^(\s*app\.kubernetes\.io/version: ).*$`), `${1}NORMALIZED`},
	// image: "<repo>:<tag>" -- the default tag derives from appVersion
	{regexp.MustCompile(`(?m)^(\s*image: ".*:)[^"]*(")\s*$`), `${1}NORMALIZED${2}`},
	// KGW_DEFAULT_IMAGE_TAG env value, also derived from appVersion
	{regexp.MustCompile(`(?m)^(\s*- name: KGW_DEFAULT_IMAGE_TAG\n\s*value: ).*$`), `${1}NORMALIZED`},
}

func normalizeChartVersionFields(rendered string) string {
	for _, n := range chartVersionFieldNormalizers {
		rendered = n.re.ReplaceAllString(rendered, n.repl)
	}
	return rendered
}

// renderChartAtPath runs `helm template` against an arbitrary chart path (a
// source directory or a packaged .tgz) and returns the rendered manifests.
func renderChartAtPath(t *testing.T, chartPath, valuesYAML string, apiVersions []string) []byte {
	t.Helper()

	_, err := os.Stat(chartPath)
	require.NoError(t, err, "chart not found at %s", chartPath)

	args := []string{"template", "test-release", chartPath, "--namespace", "default"}
	for _, apiVersion := range apiVersions {
		args = append(args, "--api-versions", apiVersion)
	}

	if valuesYAML != "" {
		valuesFile, err := os.CreateTemp("", "values-*.yaml")
		require.NoError(t, err, "failed to create temp values file")
		defer os.Remove(valuesFile.Name())

		_, err = valuesFile.WriteString(valuesYAML)
		require.NoError(t, err, "failed to write values file")
		require.NoError(t, valuesFile.Close(), "failed to close values file")

		args = append(args, "-f", valuesFile.Name())
	}

	helmCmd := helmCommand(args...)
	var output, stderr bytes.Buffer
	helmCmd.Stdout = &output
	helmCmd.Stderr = &stderr
	require.NoError(t, helmCmd.Run(), "helm template failed: %s", stderr.String())

	return output.Bytes()
}

func envTruthy(key string) bool {
	switch strings.ToLower(os.Getenv(key)) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}
