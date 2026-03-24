package buildtools

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
)

func TestDockerfileVersionsMatchGoMod(t *testing.T) {
	t.Parallel()

	rootDir := repoRoot(t)

	dockerfilePath := filepath.Join(rootDir, "tools", "build-tools", "Dockerfile")
	dockerfileBytes, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	dockerfile := string(dockerfileBytes)

	// Go and Helm versions are extracted directly from go.mod at Docker build
	// time, so there are no hardcoded values to drift. Verify the Dockerfile
	// does NOT contain stale hardcoded versions.
	if regexp.MustCompile(`(?m)^ARG GO_VERSION=`).FindStringIndex(dockerfile) != nil {
		t.Fatalf("Dockerfile should not hardcode ARG GO_VERSION; the Go version is derived from go.mod at build time")
	}
	if regexp.MustCompile(`(?m)^ENV HELM_VERSION=`).FindStringIndex(dockerfile) != nil {
		t.Fatalf("Dockerfile should not hardcode ENV HELM_VERSION; the Helm version is derived from go.mod at build time")
	}

	t.Run("kind", func(t *testing.T) {
		t.Parallel()

		// Build-tools image should not pin/download kind directly: it should use a wrapper script
		// that execs `go tool kind`, which is pinned via go.mod.
		if regexp.MustCompile(`(?m)^ENV KIND_VERSION=`).FindStringIndex(dockerfile) != nil {
			t.Fatalf("KIND_VERSION drift risk detected: Dockerfile should not set ENV KIND_VERSION")
		}
		if regexp.MustCompile(`(?m)^\s*curl\b.*\bkind\b`).FindStringIndex(dockerfile) != nil {
			t.Fatalf("KIND_VERSION drift risk detected: Dockerfile should not download kind via curl")
		}

		// KIND_VERSION in the Makefile is derived from go.mod at make time,
		// so there is no hardcoded literal to drift. Verify it stays dynamic.
		makefilePath := filepath.Join(rootDir, "Makefile")
		makefileBytes, err := os.ReadFile(makefilePath)
		if err != nil {
			t.Fatalf("read Makefile: %v", err)
		}
		makefile := string(makefileBytes)
		if regexp.MustCompile(`(?m)^KIND_VERSION\s*\?=\s*v[\d.]+\s*$`).FindStringIndex(makefile) != nil {
			t.Fatalf("KIND_VERSION drift risk detected: Makefile should derive KIND_VERSION from go.mod, not hardcode it")
		}
	})
}

func TestToolsGoModVersionMatchesRoot(t *testing.T) {
	t.Parallel()

	rootDir := repoRoot(t)

	goVersion := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		m := regexp.MustCompile(`(?m)^go\s+(\S+)\s*$`).FindSubmatch(data)
		if m == nil {
			t.Fatalf("no 'go' directive found in %s", path)
		}
		return string(m[1])
	}

	rootVersion := goVersion(filepath.Join(rootDir, "go.mod"))
	toolsVersion := goVersion(filepath.Join(rootDir, "tools", "go.mod"))

	if rootVersion != toolsVersion {
		t.Errorf("go version mismatch: go.mod has %q but tools/go.mod has %q; they must be kept in sync", rootVersion, toolsVersion)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}

	dir := filepath.Dir(thisFile)
	for i := 0; i < 20; i++ {
		// This repo has a nested Go module in `tools/`. We want the *repo* root,
		// not the tools module root, so require a Makefile alongside go.mod.
		if fileExists(filepath.Join(dir, "go.mod")) && fileExists(filepath.Join(dir, "Makefile")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Fatalf("could not locate repo root (go.mod + Makefile) starting from %q", filepath.Dir(thisFile))
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
