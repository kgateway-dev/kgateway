package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/rotisserie/eris"

	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/gloo/test/kubernetes/testutils/actions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
	testruntime "github.com/solo-io/gloo/test/kubernetes/testutils/runtime"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/go-utils/contextutils"
)

// MustTestHelper returns the SoloTestHelper used for e2e tests
// The SoloTestHelper is a wrapper around `glooctl` and we should eventually phase it out
// in favor of using the exact tool that users rely on
func MustTestHelper(ctx context.Context, installation *TestInstallation) *helper.SoloTestHelper {
	testHelper, err := kube2e.GetTestHelperForRootDir(ctx, testutils.GitRootDirectory(), installation.Metadata.InstallNamespace)
	if err != nil {
		panic(err)
	}

	testHelper.DeployTestServer = false

	return testHelper
}

func MustTestCluster() *TestCluster {
	runtimeContext := testruntime.NewContext()
	clusterContext := cluster.MustKindContext(runtimeContext.ClusterName)

	return &TestCluster{
		RuntimeContext: runtimeContext,
		ClusterContext: clusterContext,
	}
}

// TestCluster is the structure around a set of tests that run against a Kubernetes Cluster
// Within a TestCluster, we spin off multiple TestInstallation to test the behavior of a particular installation
type TestCluster struct {
	// RuntimeContext contains the set of properties that are defined at runtime by whoever is invoking tests
	RuntimeContext testruntime.Context

	// ClusterContext contains the metadata about the Kubernetes Cluster that is used for this TestCluster
	ClusterContext *cluster.Context

	// activeInstallations is the set of TestInstallation that have been created for this cluster.
	// Since tests are run serially, this will only have a single entry at a time
	activeInstallations map[string]*TestInstallation
}

func (c *TestCluster) RegisterTestInstallation(t *testing.T, glooGatewayContext *gloogateway.Context) *TestInstallation {
	if c.activeInstallations == nil {
		c.activeInstallations = make(map[string]*TestInstallation, 2)
	}

	installation := &TestInstallation{
		// Create a reference to the TestCluster, and all of it's metadata
		TestCluster: c,

		// Maintain a reference to the Metadata used for this installation
		Metadata: glooGatewayContext,

		// ResourceClients are only available _after_ installing Gloo Gateway
		ResourceClients: nil,

		// Create an operations provider, and point it to the running installation
		Actions: actions.NewActionsProvider().
			WithClusterContext(c.ClusterContext).
			WithGlooGatewayContext(glooGatewayContext),

		// Create an assertions provider, and point it to the running installation
		Assertions: assertions.NewProvider(t).
			WithClusterContext(c.ClusterContext).
			WithGlooGatewayContext(glooGatewayContext),

		// GeneratedFiles contains the unique location where files generated during the execution
		// of tests against this installation will be stored
		// By creating a unique location, per TestInstallation, we guarantee isolation between TestInstallation
		GeneratedFiles: MustGeneratedFiles(glooGatewayContext.InstallNamespace),
	}
	c.activeInstallations[installation.String()] = installation

	return installation
}

func (c *TestCluster) UnregisterTestInstallation(installation *TestInstallation) {
	if err := os.RemoveAll(installation.GeneratedFiles.TempDir); err != nil {
		panic(fmt.Sprintf("Failed to remove temporary directory: %s", installation.GeneratedFiles.TempDir))
	}

	delete(c.activeInstallations, installation.String())
}

// TestInstallation is the structure around a set of tests that validate behavior for an installation
// of Gloo Gateway.
type TestInstallation struct {
	fmt.Stringer

	// TestCluster contains the properties of the TestCluster this TestInstallation is a part of
	TestCluster *TestCluster

	// Metadata contains the properties used to install Gloo Gateway
	Metadata *gloogateway.Context

	// ResourceClients is a set of clients that can manipulate resources owned by Gloo Gateway
	ResourceClients gloogateway.ResourceClients

	// Actions is the entity that creates actions that can be executed by the Operator
	Actions *actions.Provider

	// Assertions is the entity that creates assertions that can be executed by the Operator
	Assertions *assertions.Provider

	// IstioctlBinary is the path to the istioctl binary that can be used to interact with Istio
	IstioctlBinary string

	// GeneratedFiles is the collection of directories and files that this test installation _may_ create
	GeneratedFiles GeneratedFiles
}

func (i *TestInstallation) String() string {
	return i.Metadata.InstallNamespace
}

func (i *TestInstallation) InstallGlooGateway(ctx context.Context, installFn func(ctx context.Context) error) {
	if !testutils.ShouldSkipInstall() {
		err := installFn(ctx)
		i.Assertions.Require.NoError(err)
		i.Assertions.EventuallyInstallationSucceeded(ctx)
	}

	// We can only create the ResourceClients after the CRDs exist in the Cluster
	clients, err := gloogateway.NewResourceClients(ctx, i.TestCluster.ClusterContext)
	i.Assertions.Require.NoError(err)
	i.ResourceClients = clients
}

func (i *TestInstallation) UninstallGlooGateway(ctx context.Context, uninstallFn func(ctx context.Context) error) {
	if testutils.ShouldSkipInstall() {
		return
	}
	err := uninstallFn(ctx)
	i.Assertions.Require.NoError(err)
	i.Assertions.EventuallyUninstallationSucceeded(ctx)
}

// PreFailHandler is the function that is invoked if a test in the given TestInstallation fails
func (i *TestInstallation) PreFailHandler(ctx context.Context) {
	// This is a work in progress
	// The idea here is we want to accumulate ALL information about this TestInstallation into a single directory
	// That way we can upload it in CI, or inspect it locally
	logFile := filepath.Join(i.GeneratedFiles.FailureDir, "gloo.txt")
	logsCmd := i.Actions.Kubectl().Command(ctx, "logs", "-n", i.Metadata.InstallNamespace, "deployments/gloo", ">", logFile)
	_ = logsCmd.Run()
}

const (
	IstioctlVersionEnv  = "ISTIOCTL_VERSION"
	defaultIstioVersion = "1.19.9"
)

func (i *TestInstallation) AddIstioctl(
	ctx context.Context) error {
	// Download istioctl binary
	istioctlBinary, err := DownloadIstio(ctx, getIstioctlVersionOrDefault())
	if err != nil {
		return fmt.Errorf("failed to download istio: %w", err)
	}
	contextutils.LoggerFrom(ctx).Infof("Using Istio binary '%s'", istioctlBinary)

	i.IstioctlBinary = istioctlBinary
	return nil
}

func (i *TestInstallation) InstallMinimalIstio(
	ctx context.Context) error {
	return i.InstallIstioOperator(ctx, "")
}

func (i *TestInstallation) InstallIstioOperator(
	ctx context.Context,
	operatorFile string) error {
	if testutils.ShouldSkipIstioInstall() {
		return nil
	}

	var cmd *exec.Cmd
	if operatorFile == "" {
		// use the minimal profile by default if no operator file is provided
		// yes | istioctl install --context <kube-context> --set profile=minimal
		cmd = exec.Command("sh", "-c", "yes | "+i.IstioctlBinary+" install --context "+i.TestCluster.ClusterContext.KubeContext+" --set profile=minimal")
	} else {
		cmd = exec.Command("sh", "-c", "yes | "+i.IstioctlBinary, "install", "-y", "--context", i.TestCluster.ClusterContext.KubeContext, "-f", operatorFile)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("istioctl install failed: %w", err)
	}

	return ctx.Err()
}

// GeneratedFiles is a collection of files that are generated during the execution of a set of tests
type GeneratedFiles struct {
	// TempDir is the directory where any temporary files should be created
	// Tests may create files for any number of reasons:
	// - A: When a test renders objects in a file, and then uses this file to create and delete values
	// - B: When a test invokes a command that produces a file as a side effect (glooctl, for example)
	// Files in this directory are an implementation detail of the test itself.
	// As a result, it is the callers responsibility to clean up the TempDir when the tests complete
	TempDir string

	// FailureDir is the directory where any assets that are produced on failure will be created
	FailureDir string
}

// MustGeneratedFiles returns GeneratedFiles, or panics if there was an error generating the directories
func MustGeneratedFiles(tmpDirId string) GeneratedFiles {
	tmpDir, err := os.MkdirTemp("", tmpDirId)
	if err != nil {
		panic(err)
	}

	failureDir := filepath.Join(testruntime.PathToBugReport(), tmpDirId)
	err = os.MkdirAll(failureDir, os.ModePerm)
	if err != nil {
		panic(err)
	}

	return GeneratedFiles{
		TempDir:    tmpDir,
		FailureDir: failureDir,
	}
}

func getIstioctlVersionOrDefault() string {
	if version := os.Getenv(IstioctlVersionEnv); version != "" {
		return version
	} else {
		return defaultIstioVersion
	}
}

// Download istioctl binary from istio.io/downloadIstio and returns the path to the binary
func DownloadIstio(ctx context.Context, version string) (string, error) {
	if version == "" {
		contextutils.LoggerFrom(ctx).Infof("ISTIOCTL_VERSION not specified, using istioctl from PATH")
		binaryPath, err := exec.LookPath("istioctl")
		if err != nil {
			return "", eris.New("ISTIOCTL_VERSION environment variable must be specified or istioctl must be installed")
		}

		contextutils.LoggerFrom(ctx).Infof("using istioctl path: %s", binaryPath)

		return binaryPath, nil
	}
	installLocation := filepath.Join(testutils.GitRootDirectory(), ".bin")
	binaryDir := filepath.Join(installLocation, fmt.Sprintf("istio-%s", version), "bin")
	binaryLocation := filepath.Join(binaryDir, "istioctl")

	fileInfo, _ := os.Stat(binaryLocation)
	if fileInfo != nil {
		return binaryLocation, nil
	}
	if err := os.MkdirAll(binaryDir, 0755); err != nil {
		return "", eris.Wrap(err, "create directory")
	}

	if istioctlDownloadFrom := os.Getenv("ISTIOCTL_DOWNLOAD_FROM"); istioctlDownloadFrom != "" {
		osName := "linux"
		if runtime.GOOS == "darwin" {
			osName = "osx"
		}

		arch := runtime.GOARCH
		archModifier := fmt.Sprintf("-%s", arch)

		if osName == "osx" && arch != "arm64" {
			archModifier = ""
		}

		url := fmt.Sprintf("%s/%s/istioctl-%s-%s%s.tar.gz", istioctlDownloadFrom, version, version, osName, archModifier)

		// Use curl and tar to download and extract the file
		cmd := exec.Command("sh", "-c", fmt.Sprintf("curl -sSL %s | tar -xz -C %s", url, binaryDir))
		if err := cmd.Run(); err != nil {
			return "", eris.Wrapf(err, "download and extract istioctl, cmd: %s", cmd.Args)
		}
		// Change permissions
		if err := os.Chmod(binaryLocation, 0755); err != nil {
			return "", eris.Wrap(err, "change permissions")
		}
		return binaryLocation, nil
	}

	req, err := http.NewRequest(http.MethodGet, "https://istio.io/downloadIstio", nil)
	if err != nil {
		return "", err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	cmd := exec.Command("sh", "-")

	cmd.Env = append(cmd.Env, fmt.Sprintf("ISTIO_VERSION=%s", version))
	cmd.Dir = installLocation

	cmd.Stdin = res.Body
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err = cmd.Run(); err != nil {
		return "", err
	}

	return binaryLocation, err
}

func (i *TestInstallation) UninstallIstio() error {
	// sh -c yes | istioctl uninstall —purge —context <kube-context>
	cmd := exec.Command("sh", "-c", fmt.Sprintf("yes | %s uninstall --purge --context %s", i.IstioctlBinary, i.TestCluster.ClusterContext.KubeContext))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("istioctl uninstall failed: %w", err)
	}
	return nil
}
