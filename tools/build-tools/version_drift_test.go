package buildtools

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
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

func TestIstioVersionDefaultsDoNotDrift(t *testing.T) {
	t.Parallel()

	rootDir := repoRoot(t)

	deployerPath := filepath.Join(rootDir, "pkg", "deployer", "gateway_parameters.go")
	prTestVersionsPath := filepath.Join(rootDir, ".github", "workflows", ".env", "pr-tests", "versions.env")
	nightlyMaxVersionsPath := filepath.Join(rootDir, ".github", "workflows", ".env", "nightly-tests", "max_versions.env")
	e2eRuntimePath := filepath.Join(rootDir, "test", "e2e", "testutils", "runtime", "istio_version.go")

	deployerTag := goStringConst(t, deployerPath, "DefaultIstioProxyImageTag")
	prTestVersion := envValue(t, prTestVersionsPath, "istio_version")
	nightlyMaxVersion := envValue(t, nightlyMaxVersionsPath, "istio_version")
	e2eDefaultVersion := goStringConst(t, e2eRuntimePath, "DefaultIstioVersion")

	for name, version := range map[string]string{
		"PR test istio_version":     prTestVersion,
		"nightly max istio_version": nightlyMaxVersion,
		"e2e DefaultIstioVersion":   e2eDefaultVersion,
	} {
		if version != deployerTag {
			t.Errorf(
				"%s = %q, want %q to match deployer DefaultIstioProxyImageTag",
				name, version, deployerTag,
			)
		}
	}

	proxyV2TagPattern := regexp.MustCompile(`proxyv2:([A-Za-z0-9._-]+)`)
	testdataDir := filepath.Join(rootDir, "test", "deployer", "testdata")
	err := filepath.WalkDir(testdataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, "-out.yaml") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range proxyV2TagPattern.FindAllSubmatch(data, -1) {
			tag := string(match[1])
			if tag != deployerTag {
				t.Errorf("%s contains Istio proxy image tag %q, want %q", path, tag, deployerTag)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", testdataDir, err)
	}
}

func TestEnvoyVersionTracksIstioVersion(t *testing.T) {
	t.Parallel()

	rootDir := repoRoot(t)

	deployerPath := filepath.Join(rootDir, "pkg", "deployer", "gateway_parameters.go")
	makefilePath := filepath.Join(rootDir, "Makefile")

	istioMajor, istioMinor := majorMinorVersion(t, goStringConst(t, deployerPath, "DefaultIstioProxyImageTag"))
	envoyImage := makeVarValue(t, makefilePath, "ENVOY_IMAGE")
	envoyMajor, envoyMinor := majorMinorVersion(t, imageTag(t, envoyImage))

	if envoyMajor != istioMajor || envoyMinor != istioMinor+8 {
		t.Fatalf(
			"Envoy and Istio versions must remain aligned; see issue 14011 for details: Envoy %d.%d should match Istio %d.%d plus 0.8",
			envoyMajor, envoyMinor, istioMajor, istioMinor,
		)
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

func goStringConst(t *testing.T, path, name string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	pattern := regexp.MustCompile(`(?m)^\s*(?:const\s+)?` + regexp.QuoteMeta(name) + `\s*=\s*"([^"]+)"\s*$`)
	match := pattern.FindSubmatch(data)
	if match == nil {
		t.Fatalf("could not find string constant %s in %s", name, path)
	}
	return string(match[1])
}

func envValue(t *testing.T, path, name string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	pattern := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `='([^']+)'\s*$`)
	match := pattern.FindSubmatch(data)
	if match == nil {
		t.Fatalf("could not find env value %s in %s", name, path)
	}
	return string(match[1])
}

func makeVarValue(t *testing.T, path, name string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	pattern := regexp.MustCompile(`(?m)^(?:export\s+)?` + regexp.QuoteMeta(name) + `\s*(?:\?=|=)\s*(\S+)\s*$`)
	match := pattern.FindSubmatch(data)
	if match == nil {
		t.Fatalf("could not find Makefile variable %s in %s", name, path)
	}
	return string(match[1])
}

func imageTag(t *testing.T, image string) string {
	t.Helper()

	_, tag, ok := strings.Cut(image[strings.LastIndex(image, "/")+1:], ":")
	if !ok {
		t.Fatalf("image %q has no tag", image)
	}
	return tag
}

func majorMinorVersion(t *testing.T, version string) (int, int) {
	t.Helper()

	version = strings.TrimPrefix(version, "v")
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		t.Fatalf("version %q does not include major and minor components", version)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		t.Fatalf("parse major version from %q: %v", version, err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		t.Fatalf("parse minor version from %q: %v", version, err)
	}
	return major, minor
}
