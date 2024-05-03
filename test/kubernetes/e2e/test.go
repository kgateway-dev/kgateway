package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rotisserie/eris"

	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/gloo/test/kubernetes/testutils/actions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
	k8sruntime "github.com/solo-io/gloo/test/kubernetes/testutils/runtime"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/go-utils/contextutils"
)

// MustTestHelper returns the SoloTestHelper used for e2e tests
// The SoloTestHelper is a wrapper around `glooctl` and we should eventually phase it out
// in favor of using the exact tool that users rely on
func MustTestHelper(ctx context.Context, installation *TestInstallation) *helper.SoloTestHelper {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	rootDir := filepath.Join(cwd, "../../../../")
	testHelper, err := kube2e.GetTestHelperForRootDir(ctx, rootDir, installation.Metadata.InstallNamespace)
	if err != nil {
		panic(err)
	}

	testHelper.DeployTestServer = false

	return testHelper
}

func MustTestCluster() *TestCluster {
	runtimeContext := k8sruntime.NewContext()
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
	RuntimeContext k8sruntime.Context

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
	}
	c.activeInstallations[installation.String()] = installation

	return installation
}

func (c *TestCluster) UnregisterTestInstallation(installation *TestInstallation) {
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
	logsCmd := i.Actions.Kubectl().Command(ctx, "logs", "-n", i.Metadata.InstallNamespace, "deployments/gloo")
	logsCmd.Run()
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
	installLocation := filepath.Join(GlooDirectory(), ".bin")
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

func GlooDirectory() string {
	data, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		panic(err)
	}
	return strings.TrimSpace(string(data))
}
