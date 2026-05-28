//go:build e2e

package helper

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetLocalChartPathPrefersPackagedChart(t *testing.T) {
	rootDir := t.TempDir()
	assetDir := filepath.Join(rootDir, defaultTestAssetDir)
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatalf("failed to create asset dir: %v", err)
	}

	index := `apiVersion: v1
entries:
  kgateway:
  - apiVersion: v2
    created: "2026-04-21T00:00:00Z"
    description: test chart
    digest: deadbeef
    name: kgateway
    urls:
    - kgateway-v1.2.3.tgz
    version: v1.2.3
generated: "2026-04-21T00:00:00Z"
`
	if err := os.WriteFile(filepath.Join(assetDir, HelmRepoIndexFileName), []byte(index), 0o600); err != nil {
		t.Fatalf("failed to write Helm index: %v", err)
	}
	packagedChartPath := filepath.Join(assetDir, "kgateway-v1.2.3.tgz")
	if err := os.WriteFile(packagedChartPath, []byte("chart"), 0o600); err != nil {
		t.Fatalf("failed to write packaged chart: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(rootDir, defaultHelmChartDir, "kgateway"), 0o755); err != nil {
		t.Fatalf("failed to create chart dir: %v", err)
	}

	chartPath, err := getLocalChartPath(rootDir, "kgateway", "")
	if err != nil {
		t.Fatalf("expected packaged chart path, got error: %v", err)
	}
	if chartPath != packagedChartPath {
		t.Fatalf("expected packaged chart path %q, got %q", packagedChartPath, chartPath)
	}
}

func TestGetLocalChartPathFallsBackToChartDirectory(t *testing.T) {
	rootDir := t.TempDir()
	chartDir := filepath.Join(rootDir, defaultHelmChartDir, "kgateway")
	if err := os.MkdirAll(chartDir, 0o755); err != nil {
		t.Fatalf("failed to create chart dir: %v", err)
	}

	chartPath, err := getLocalChartPath(rootDir, "kgateway", "")
	if err != nil {
		t.Fatalf("expected unpackaged chart path, got error: %v", err)
	}
	if chartPath != chartDir {
		t.Fatalf("expected chart dir %q, got %q", chartDir, chartPath)
	}
}

// Common case after this PR: the asset directory exists (e.g. for bug_report
// output) but charts are not packaged. We should silently fall through to the
// unpackaged chart directory rather than attempting to parse a missing index.
func TestGetLocalChartPathFallsBackWhenAssetDirHasNoIndex(t *testing.T) {
	rootDir := t.TempDir()
	assetDir := filepath.Join(rootDir, defaultTestAssetDir)
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatalf("failed to create asset dir: %v", err)
	}
	chartDir := filepath.Join(rootDir, defaultHelmChartDir, "kgateway")
	if err := os.MkdirAll(chartDir, 0o755); err != nil {
		t.Fatalf("failed to create chart dir: %v", err)
	}

	chartPath, err := getLocalChartPath(rootDir, "kgateway", "")
	if err != nil {
		t.Fatalf("expected unpackaged chart path, got error: %v", err)
	}
	if chartPath != chartDir {
		t.Fatalf("expected chart dir %q, got %q", chartDir, chartPath)
	}
}
