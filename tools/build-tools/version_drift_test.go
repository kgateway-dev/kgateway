package buildtools

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"golang.org/x/mod/modfile"
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

	gotGoVersion := mustMatch1(t, dockerfile, `(?m)^ARG GO_VERSION=([^\s]+)\s*$`, "Dockerfile ARG GO_VERSION")
	gotHelmVersion := mustMatch1(t, dockerfile, `(?m)^ENV HELM_VERSION=([^\s]+)\s*$`, "Dockerfile ENV HELM_VERSION")

	goModPath := filepath.Join(rootDir, "go.mod")
	goModBytes, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	parsed, err := modfile.Parse(goModPath, goModBytes, nil)
	if err != nil {
		t.Fatalf("parse go.mod: %v", err)
	}
	if parsed.Go == nil || parsed.Go.Version == "" {
		t.Fatalf("go.mod is missing a go version directive")
	}

	wantGoVersion := parsed.Go.Version
	wantHelmVersion := requireVersion(t, parsed, "helm.sh/helm/v3")
	wantKindVersion := requireVersion(t, parsed, "sigs.k8s.io/kind")

	makefilePath := filepath.Join(rootDir, "Makefile")
	makefileBytes, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	makefile := string(makefileBytes)
	gotKindVersion := mustMatch1(t, makefile, `(?m)^KIND_VERSION\s*\?=\s*([^\s]+)\s*$`, "Makefile KIND_VERSION")

	t.Run("go", func(t *testing.T) {
		t.Parallel()
		if gotGoVersion != wantGoVersion {
			t.Fatalf("GO_VERSION drift detected: Dockerfile has %q, go.mod has %q", gotGoVersion, wantGoVersion)
		}
	})

	t.Run("helm", func(t *testing.T) {
		t.Parallel()
		if gotHelmVersion != wantHelmVersion {
			t.Fatalf("HELM_VERSION drift detected: Dockerfile has %q, go.mod helm.sh/helm/v3 is %q", gotHelmVersion, wantHelmVersion)
		}
	})

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

		if gotKindVersion != wantKindVersion {
			t.Fatalf("KIND_VERSION drift detected: Makefile has %q, go.mod sigs.k8s.io/kind is %q", gotKindVersion, wantKindVersion)
		}
	})
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

func mustMatch1(t *testing.T, content, pattern, label string) string {
	t.Helper()

	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(content)
	if len(m) != 2 {
		t.Fatalf("%s not found (pattern %q)", label, pattern)
	}
	return m[1]
}

func requireVersion(t *testing.T, mf *modfile.File, modulePath string) string {
	t.Helper()

	for _, req := range mf.Require {
		if req.Mod.Path == modulePath {
			return req.Mod.Version
		}
	}

	t.Fatalf("go.mod is missing required module %q", modulePath)
	return ""
}
