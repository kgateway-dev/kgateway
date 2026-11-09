//go:build e2e

package cluster

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestDownloadIstioRelease(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the release downloader requires POSIX tools")
	}
	for _, tc := range []struct {
		name, goos, goarch, suffix, failure string
	}{
		{name: "linux amd64", goos: "linux", goarch: "amd64", suffix: "linux-amd64"},
		{name: "linux arm64", goos: "linux", goarch: "arm64", suffix: "linux-arm64"},
		{name: "darwin amd64", goos: "darwin", goarch: "amd64", suffix: "osx"},
		{name: "darwin arm64", goos: "darwin", goarch: "arm64", suffix: "osx-arm64"},
		{name: "checksum mismatch", goos: "linux", goarch: "amd64", suffix: "linux-amd64", failure: "SHA-256 mismatch"},
		{name: "missing asset", goos: "linux", goarch: "amd64", suffix: "linux-amd64", failure: "Unable to resolve"},
		{name: "download failure", goos: "linux", goarch: "amd64", suffix: "linux-amd64", failure: "exit status 22"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const version = "1.31.0"
			const binaryContents = "#!/bin/sh\necho istioctl\n"
			dir := t.TempDir()
			archive := filepath.Join(dir, "release.tar.gz")
			writeIstioArchive(t, archive, version, binaryContents)
			data, err := os.ReadFile(archive)
			if err != nil {
				t.Fatal(err)
			}
			digest := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
			if tc.failure == "SHA-256 mismatch" {
				digest = "sha256:" + strings.Repeat("0", 64)
			}
			assetName := fmt.Sprintf("istio-%s-%s.tar.gz", version, tc.suffix)
			if tc.failure == "Unable to resolve" {
				assetName = "other.tar.gz"
			}
			metadata, err := json.Marshal(map[string]any{"assets": []any{
				map[string]any{"id": 123, "name": assetName, "digest": digest},
			}})
			if err != nil {
				t.Fatal(err)
			}
			metadataPath := filepath.Join(dir, "release.json")
			if err := os.WriteFile(metadataPath, metadata, 0o600); err != nil {
				t.Fatal(err)
			}
			fakeBin := filepath.Join(dir, "tools")
			if err := os.Mkdir(fakeBin, 0o755); err != nil {
				t.Fatal(err)
			}
			// Exercise the real downloader and checksum tools, replacing only HTTP.
			curl := `#!/bin/sh
printf '%s\n' "$@" >> "$TEST_DOWNLOAD_LOG"
case "$*" in
  *https://api.github.com/repos/istio/istio/releases/tags/1.31.0*)
    cat "$TEST_RELEASE_METADATA" ;;
  *https://api.github.com/repos/istio/istio/releases/assets/123*)
    if [ "$TEST_DOWNLOAD_FAILURE" = true ]; then exit 22; fi
    while [ "$#" -gt 0 ]; do
      if [ "$1" = --output ]; then cp "$TEST_RELEASE_ARCHIVE" "$2"; exit; fi
      shift
    done
    exit 2 ;;
  *) echo "unexpected URL" >&2; exit 2 ;;
esac
`
			curlPath := filepath.Join(fakeBin, "curl")
			if err := os.WriteFile(curlPath, []byte(curl), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(curlPath, 0o700); err != nil {
				t.Fatal(err)
			}
			requestLog := filepath.Join(dir, "requests")
			t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("GITHUB_TOKEN", "test-token")
			t.Setenv("TEST_DOWNLOAD_LOG", requestLog)
			t.Setenv("TEST_RELEASE_METADATA", metadataPath)
			t.Setenv("TEST_RELEASE_ARCHIVE", archive)
			t.Setenv("TEST_DOWNLOAD_FAILURE", strconv.FormatBool(tc.failure == "exit status 22"))
			// Start without either the install root or the binary's parent directory.
			installLocation := filepath.Join(dir, "new", "install")
			binary := filepath.Join(installLocation, "istio-"+version, "bin", "istioctl")

			err = downloadIstioRelease(t.Context(), version, installLocation, tc.goos, tc.goarch)
			if tc.failure != "" {
				if err == nil || !strings.Contains(err.Error(), tc.failure) {
					t.Fatalf("expected %q error, got %v", tc.failure, err)
				}
				if _, err := os.Stat(binary); !os.IsNotExist(err) {
					t.Fatalf("failed download must not cache a binary: %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				installed, err := os.ReadFile(binary)
				if err != nil || string(installed) != binaryContents {
					t.Fatalf("unexpected installed binary %q: %v", installed, err)
				}
				info, err := os.Stat(binary)
				if err != nil || info.Mode().Perm() != 0o755 {
					t.Fatalf("binary must be executable: %v", err)
				}
				requests, err := os.ReadFile(requestLog)
				if err != nil {
					t.Fatal(err)
				}
				for _, required := range []string{"Accept: application/octet-stream", "Authorization: Bearer test-token"} {
					if !strings.Contains(string(requests), required) {
						t.Errorf("API requests missing %q", required)
					}
				}
			}
			entries, err := filepath.Glob(filepath.Join(installLocation, "istio-download-*"))
			if err != nil || len(entries) != 0 {
				t.Fatalf("temporary downloads must be removed: %v, %v", entries, err)
			}
		})
	}
}

func TestUninstallIstioWithoutBinary(t *testing.T) {
	if err := UninstallIstio("", "unused"); err != nil {
		t.Fatalf("cleanup before obtaining istioctl should do nothing: %v", err)
	}
}

func writeIstioArchive(t *testing.T, filename, version, contents string) {
	t.Helper()
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name: "istio-" + version + "/bin/istioctl",
		Mode: 0o755,
		Size: int64(len(contents)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	for _, close := range []func() error{tw.Close, gz.Close, file.Close} {
		if err := close(); err != nil {
			t.Fatal(err)
		}
	}
}
