//go:build e2e

package cluster

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	testruntime "github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/runtime"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var logger = logging.New("e2e/cluster")

const (
	// TODO(npolshak): Add support for other profiles (ambient, etc.)
	minimalProfile = "minimal"
)

func GetIstioctl(ctx context.Context) (string, error) {
	// Download istioctl binary
	istioctlBinary, err := downloadIstio(ctx, getIstioVersion())
	if err != nil {
		return "", fmt.Errorf("failed to download istio: %w", err)
	}
	logger.Info("using Istio binary", "path", istioctlBinary)

	return istioctlBinary, nil
}

func InstallMinimalIstio(
	ctx context.Context,
	istioctlBinary, kubeContext string,
) error {
	operatorFileContent := generateIstioOperatorFileContent("", minimalProfile)
	operatorFile := filepath.Join(os.TempDir(), "istio-operator.yaml")

	err := os.WriteFile(operatorFile, []byte(operatorFileContent), 0o600)
	if err != nil {
		return fmt.Errorf("failed to write operator file: %w", err)
	}

	return installIstioOperator(ctx, istioctlBinary, kubeContext, operatorFile)
}

func InstallRevisionedIstio(
	ctx context.Context,
	istioctlBinary, kubeContext, revision, profile string,
	extraArgs ...string,
) error {
	operatorFileContent := generateIstioOperatorFileContent(revision, profile)
	operatorFile := filepath.Join(os.TempDir(), "istio-operator.yaml")

	err := os.WriteFile(operatorFile, []byte(operatorFileContent), 0o600)
	if err != nil {
		return fmt.Errorf("failed to write operator file: %w", err)
	}

	return installIstioOperator(ctx, istioctlBinary, kubeContext, operatorFile, extraArgs...)
}

// TODO(npolshak): Add Istio dependency to define operator in code instead of writing file
func generateIstioOperatorFileContent(revision, profile string) string {
	// use minimal as the default profile if none is provided
	if profile == "" {
		profile = minimalProfile
	}

	baseContent := `
apiVersion: install.istio.io/v1alpha1
kind: IstioOperator
metadata:
  namespace: istio-system
spec:
  profile: %s
`
	if revision != "" {
		return fmt.Sprintf(baseContent+"  revision: %s\n", profile, revision)
	}
	return fmt.Sprintf(baseContent, profile)
}

func installIstioOperator(
	ctx context.Context,
	istioctlBinary, kubeContext, operatorFile string,
	extraArgs ...string,
) error {
	if testutils.ShouldSkipIstioInstall() {
		return nil
	}

	//  istioctl install -y --context <kube-context> -f <operator-file>
	args := append([]string{
		"install", "-y", "--context", kubeContext, "-f", operatorFile,
	}, extraArgs...)
	cmd := exec.Command(istioctlBinary, args...)

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("istioctl install failed: %w\noutput:\n%s", err, string(out))
	}

	return ctx.Err()
}

func getIstioVersion() string {
	if version := os.Getenv(testruntime.IstioVersionEnv); version != "" {
		return version
	} else {
		// Fail loudly if ISTIO_VERSION is not set
		panic(testruntime.IstioVersionEnv + " environment variable must be specified to run")
	}
}

// downloadIstio downloads the requested release and returns the istioctl binary path.
func downloadIstio(ctx context.Context, version string) (string, error) {
	if version == "" {
		logger.Info("ISTIO_VERSION not specified, using istioctl from PATH")
		binaryPath, err := exec.LookPath("istioctl")
		if err != nil {
			return "", errors.New("ISTIO_VERSION environment variable must be specified or istioctl must be installed")
		}

		logger.Info("using istioctl", "path", binaryPath)

		return binaryPath, nil
	}
	installLocation := filepath.Join(testutils.GitRootDirectory(), ".bin")
	binaryDir := filepath.Join(installLocation, "istio-"+version, "bin")
	binaryLocation := filepath.Join(binaryDir, "istioctl")

	fileInfo, _ := os.Stat(binaryLocation)
	if fileInfo != nil {
		return binaryLocation, nil
	}
	if err := os.MkdirAll(binaryDir, 0o755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}

	if istioctlDownloadFrom := os.Getenv("ISTIOCTL_DOWNLOAD_FROM"); istioctlDownloadFrom != "" {
		osName := "linux"
		if runtime.GOOS == "darwin" {
			osName = "osx"
		}

		arch := runtime.GOARCH
		archModifier := "-" + arch

		if osName == "osx" && arch != "arm64" {
			archModifier = ""
		}

		url := fmt.Sprintf("%s/%s/istioctl-%s-%s%s.tar.gz", istioctlDownloadFrom, version, version, osName, archModifier)

		// Use curl and tar to download and extract the file
		cmd := exec.Command("sh", "-c", fmt.Sprintf("curl -sSL %s | tar -xz -C %s", url, binaryDir)) //nolint:gosec // G204: controlled download command for istioctl binary in tests
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("download and extract istioctl, cmd: %s: %w", cmd.Args, err)
		}
		// Change permissions
		if err := os.Chmod(binaryLocation, 0o755); err != nil {
			return "", fmt.Errorf("change permissions: %w", err)
		}
		return binaryLocation, nil
	}

	if err := downloadIstioRelease(ctx, version, installLocation, runtime.GOOS, runtime.GOARCH); err != nil {
		return "", err
	}

	return binaryLocation, nil
}

func downloadIstioRelease(ctx context.Context, version, installLocation, goos, goarch string) error {
	if err := os.MkdirAll(installLocation, 0o755); err != nil {
		return fmt.Errorf("create Istio install directory: %w", err)
	}
	tempDir, err := os.MkdirTemp(installLocation, "istio-download-*")
	if err != nil {
		return fmt.Errorf("create Istio download directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	osName := "linux"
	archSuffix := "-" + goarch
	if goos == "darwin" {
		osName = "osx"
		if goarch != "arm64" {
			archSuffix = ""
		}
	}
	assetName := fmt.Sprintf("istio-%s-%s%s.tar.gz", version, osName, archSuffix)
	archive := filepath.Join(tempDir, assetName)
	// The upstream installer probes /releases/download/ and calls every HTTP
	// failure "not found". Use the asset API and verify its SHA-256 instead.
	downloader := filepath.Join(testutils.GitRootDirectory(), "hack", "download-github-release-asset.sh")
	cmd := exec.CommandContext(ctx, downloader, "istio/istio", version, assetName, archive)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("download Istio release: %w\n%s", err, out)
	}

	member := filepath.Join("istio-"+version, "bin", "istioctl")
	cmd = exec.CommandContext(ctx, "tar", "-xzf", archive, "-C", tempDir, member)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("extract istioctl: %w\n%s", err, out)
	}
	binary := filepath.Join(tempDir, member)
	if err := os.Chmod(binary, 0o755); err != nil {
		return fmt.Errorf("change istioctl permissions: %w", err)
	}
	// Cache only a successfully extracted binary, so a failed attempt can be retried.
	destination := filepath.Join(installLocation, member)
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create istioctl cache directory: %w", err)
	}
	if err := os.Rename(binary, destination); err != nil {
		return fmt.Errorf("cache istioctl: %w", err)
	}
	return nil
}

func UninstallIstio(istioctlBinary, kubeContext string) error {
	// Setup can fail before a binary is available, including during download.
	if istioctlBinary == "" {
		return nil
	}
	// sh -c yes | istioctl uninstall —purge —context <kube-context>
	cmd := exec.Command("sh", "-c", fmt.Sprintf("yes | %s uninstall --purge --context '%s'", istioctlBinary, kubeContext)) //nolint:gosec // G204: controlled istioctl uninstall command in tests
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	// istioctl ends with "✔ Uninstall complete" and no trailing newline. Terminate it so the
	// next "--- PASS"/"--- FAIL" line starts its own line; otherwise test2json drops the results.
	fmt.Fprintln(os.Stdout)
	if err != nil {
		return fmt.Errorf("istioctl uninstall failed: %w", err)
	}
	return nil
}

// CreateIstioBugReport generates an istioctl bug report and moves it to the _output directory
func CreateIstioBugReport(ctx context.Context, istioctlBinary, kubeContext, artifactOutputDir string) {
	// Generate istioctl bug report
	if istioctlBinary == "" {
		log.Fatal("istioctl binary not set. Cannot generate istioctl bug report.")
	}

	bugReportCmd := exec.Command(istioctlBinary, "bug-report", "--full-secrets", "--context", kubeContext)
	bugReportErr := bugReportCmd.Run()
	if bugReportErr != nil {
		fmt.Println("Error generating bug report:", bugReportErr)
	}
	mvCmd := exec.Command("mv", "bug-report.tar.gz", artifactOutputDir)
	mvErr := mvCmd.Run()
	if mvErr != nil {
		fmt.Println("Error moving bug report file:", mvErr)
	}
}
