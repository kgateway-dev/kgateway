package helper

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"helm.sh/helm/v3/pkg/repo"

	"github.com/pkg/errors"
	"github.com/spf13/afero"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils/fsutils"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/testutils/exec"
)

const (
	GATEWAY = "gateway"
)

// Default test configuration
var defaults = TestConfig{
	TestAssetDir:          "_test",
	BuildAssetDir:         "_output",
	HelmRepoIndexFileName: "index.yaml",
	DeployTestServer:      true,
}

// supportedArchs is represents the list of architectures we build glooctl for
var supportedArchs = map[string]struct{}{
	"arm64": {},
	"amd64": {},
}

// returns true if supported, based on `supportedArchs`
func isSupportedArch() (string, bool) {
	if goarch, ok := os.LookupEnv("GOARCH"); ok {
		// if the environment's goarch is supported
		_, ok := supportedArchs[goarch]
		return goarch, ok
	}

	// if the runtime's goarch is supported
	runtimeArch := runtime.GOARCH
	_, ok := supportedArchs[runtimeArch]
	return runtimeArch, ok
}

// Function to provide/override test configuration. Default values will be passed in.
type TestConfigFunc func(defaults TestConfig) TestConfig

type TestConfig struct {
	// All relative paths will assume this as the base directory. This is usually the project base directory.
	RootDir string
	// The directory holding the test assets. Must be relative to RootDir.
	TestAssetDir string
	// The directory holding the build assets. Must be relative to RootDir.
	BuildAssetDir string
	// Helm chart name
	HelmChartName string
	// Name of the helm index file name
	HelmRepoIndexFileName string
	// The namespace gloo (and the test server) will be installed to. If empty, will use the helm chart version.
	InstallNamespace string
	// Name of the glooctl executable
	GlooctlExecName string
	// If provided, the licence key to install the enterprise version of Gloo
	LicenseKey string
	// Determines whether the test server pod gets deployed
	DeployTestServer bool
	// Install a released version of gloo. This is the value of the github tag that may have a leading 'v'
	ReleasedVersion string
	// If true, glooctl will be run with a -v flag
	Verbose bool

	// The version of the Helm chart. Calculated from either the chart or the released version. It will not have a leading 'v'
	version string
}

type SoloTestHelper struct {
	*TestConfig
	// The kubernetes helper
	*kubectl.Cli
}

// NewSoloTestHelper is meant to provide a standard way of deploying Gloo/GlooE to a k8s cluster during tests.
// It assumes that build and test assets are present in the `_output` and `_test` directories (these are configurable).
// Specifically, it expects the glooctl executable in the BuildAssetDir and a helm chart in TestAssetDir.
// It also assumes that a kubectl executable is on the PATH.
func NewSoloTestHelper(configFunc TestConfigFunc) (*SoloTestHelper, error) {

	// Get and validate test config
	testConfig := defaults
	if configFunc != nil {
		testConfig = configFunc(defaults)
	}
	// Depending on the testing tool used, GOARCH may always be set if not set already by detecting the local arch
	// (`go test`), `ginkgo` and other testing tools may not do this requiring keeping the runtime.GOARCH check
	if testConfig.GlooctlExecName == "" {
		if arch, ok := isSupportedArch(); ok {
			testConfig.GlooctlExecName = "glooctl-" + runtime.GOOS + "-" + arch
		} else {
			testConfig.GlooctlExecName = "glooctl-" + runtime.GOOS + "-amd64"
		}
	}

	// Get chart version
	if testConfig.ReleasedVersion == "" {
		if err := validateConfig(testConfig); err != nil {
			return nil, errors.Wrapf(err, "test config validation failed")
		}
		version, err := getChartVersion(testConfig)
		if err != nil {
			return nil, errors.Wrapf(err, "getting Helm chart version")
		}
		testConfig.version = version
	} else {
		// we use the version field as a chart version and tests assume it doesn't have a leading 'v'
		if testConfig.ReleasedVersion[0] == 'v' {
			testConfig.version = testConfig.ReleasedVersion[1:]
		} else {
			testConfig.version = testConfig.ReleasedVersion
		}
	}
	// Default the install namespace to the chart version.
	// Currently the test chart version built in CI contains the build id, so the namespace will be unique).
	if testConfig.InstallNamespace == "" {
		testConfig.InstallNamespace = testConfig.version
	}

	testHelper := &SoloTestHelper{
		TestConfig: &testConfig,
	}

	// Optionally, initialize a test server
	// this should not be done here but managed by tests requiring this applying the manifest
	if testConfig.DeployTestServer {
		err := testHelper.Cli.ApplyFile(context.TODO(), filepath.Join(testConfig.RootDir, "test", "kubernetes", "e2e", "defaults", "testdata", "nginx_pod.yaml"))
		if err != nil {
			return nil, errors.Wrapf(err, "initializing test nginx upstream")
		}
	}

	return testHelper, nil
}

func (h *SoloTestHelper) SetKubeCli(cli *kubectl.Cli) {
	h.Cli = cli
}

// Return the version of the Helm chart
func (h *SoloTestHelper) ChartVersion() string {
	return h.version
}

type InstallOption func(*InstallOptions)

type InstallOptions struct {
	GlooctlCommand []string
	Verbose        bool
}

func ExtraArgs(args ...string) func(*InstallOptions) {
	return func(io *InstallOptions) {
		io.GlooctlCommand = append(io.GlooctlCommand, args...)
	}
}

// Installs Gloo (and, optionally, the test server)
func (h *SoloTestHelper) InstallGloo(ctx context.Context, deploymentType string, timeout time.Duration, options ...InstallOption) error {
	log.Printf("installing gloo in [%s] mode to namespace [%s]", deploymentType, h.InstallNamespace)
	glooctlCommand := []string{
		filepath.Join(h.BuildAssetDir, h.GlooctlExecName),
		"install", deploymentType,
	}
	if h.LicenseKey != "" {
		glooctlCommand = append(glooctlCommand, "enterprise", "--license-key", h.LicenseKey)
	}
	if h.ReleasedVersion != "" {
		glooctlCommand = append(glooctlCommand, "-n", h.InstallNamespace, "--version", h.ReleasedVersion)
	} else {
		glooctlCommand = append(glooctlCommand,
			"-n", h.InstallNamespace,
			"-f", filepath.Join(h.TestAssetDir, h.HelmChartName+"-"+h.version+".tgz"))
	}
	if h.Verbose {
		glooctlCommand = append(glooctlCommand, "-v")
	}
	variant := os.Getenv("IMAGE_VARIANT")
	if variant != "" {
		variantValuesFile, err := GenerateVariantValuesFile(variant)
		if err != nil {
			return err
		}
		options = append(options, ExtraArgs("--values", variantValuesFile))
	}

	io := &InstallOptions{
		GlooctlCommand: glooctlCommand,
		Verbose:        true,
	}
	for _, opt := range options {
		opt(io)
	}

	if err := glooctlInstallWithTimeout(h.RootDir, io, time.Minute*2); err != nil {
		return errors.Wrapf(err, "error running glooctl install command")
	}

	return nil
}

// Wait for the glooctl install command to respond, err on timeout.
// The command returns as soon as certgen completes and all other
// deployments have been applied, which should only be delayed if
// there's an issue pulling the certgen docker image.
// Without this timeout, it would just hang indefinitely.
func glooctlInstallWithTimeout(rootDir string, io *InstallOptions, timeout time.Duration) error {
	runResponse := make(chan error, 1)
	go func() {
		err := exec.RunCommand(rootDir, io.Verbose, io.GlooctlCommand...)
		if err != nil {
			runResponse <- errors.Wrapf(err, "error while installing gloo")
		}
		runResponse <- nil
	}()

	select {
	case err := <-runResponse:
		return err // can be nil
	case <-time.After(timeout):
		return errors.New("timeout - did something go wrong fetching the docker images?")
	}
}

// passes the --all flag to glooctl uninstall
func (h *SoloTestHelper) UninstallGlooAll() error {
	return h.uninstallGloo(true)
}

// does not pass the --all flag to glooctl uninstall
func (h *SoloTestHelper) UninstallGloo() error {
	return h.uninstallGloo(false)
}

func (h *SoloTestHelper) uninstallGloo(all bool) error {
	if h.DeployTestServer {
		h.DeleteFile(context.TODO(), filepath.Join(h.RootDir, "test", "kubernetes", "e2e", "defaults", "testdata", "nginx_pod.yaml"))
	}

	log.Printf("uninstalling gloo...")
	cmdArgs := []string{
		filepath.Join(h.BuildAssetDir, h.GlooctlExecName), "uninstall", "-n", h.InstallNamespace, "--delete-namespace",
	}
	if all {
		cmdArgs = append(cmdArgs, "--all")
	}
	return exec.RunCommand(h.RootDir, true, cmdArgs...)
}

// Parses the Helm index file and returns the version of the chart.
func getChartVersion(config TestConfig) (string, error) {

	// Find helm index file in test asset directory
	helmIndexFile := filepath.Join(config.RootDir, config.TestAssetDir, config.HelmRepoIndexFileName)
	helmIndex, err := repo.LoadIndexFile(helmIndexFile)
	if err != nil {
		return "", errors.Wrapf(err, "parsing Helm index file")
	}
	log.Printf("found Helm index file at: %s", helmIndexFile)

	// Read and return version from helm index file
	if chartVersions, ok := helmIndex.Entries[config.HelmChartName]; !ok {
		return "", eris.Errorf("index file does not contain entry with key: %s", config.HelmChartName)
	} else if len(chartVersions) == 0 || len(chartVersions) > 1 {
		return "", eris.Errorf("expected a single entry with name [%s], found: %v", config.HelmChartName, len(chartVersions))
	} else {
		version := chartVersions[0].Version
		log.Printf("version of [%s] Helm chart is: %s", config.HelmChartName, version)
		return version, nil
	}
}

func validateConfig(config TestConfig) error {
	for _, dirName := range []string{
		config.RootDir,
		filepath.Join(config.RootDir, config.BuildAssetDir),
		filepath.Join(config.RootDir, config.TestAssetDir),
	} {
		if !fsutils.IsDirectory(dirName) {
			return fmt.Errorf("%s does not exist or is not a directory", dirName)
		}
	}
	return nil
}

func GenerateVariantValuesFile(variant string) (string, error) {
	content := `global:
  image:
    variant: ` + variant

	fs := afero.NewOsFs()
	dir, err := afero.TempDir(fs, "", "")
	if err != nil {
		return "", err
	}

	tmpFile, err := afero.TempFile(fs, dir, "")
	if err != nil {
		return "", err
	}
	_, err = tmpFile.WriteString(content)
	if err != nil {
		return "", err
	}

	return tmpFile.Name(), nil
}
