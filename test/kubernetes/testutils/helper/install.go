package helper

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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

// Default test configuration
var defaults = TestConfig{
	TestAssetDir:          "_test",
	BuildAssetDir:         "_output",
	HelmRepoIndexFileName: "index.yaml",
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

	return testHelper, nil
}

func (h *SoloTestHelper) SetKubeCli(cli *kubectl.Cli) {
	h.Cli = cli
}

// Return the version of the Helm chart
func (h *SoloTestHelper) ChartVersion() string {
	return h.version
}

// Return the path to the chart used for installation
func (h *SoloTestHelper) ChartPath() string {
	return filepath.Join(h.RootDir, h.TestAssetDir, fmt.Sprintf("%s-%s.tgz", h.HelmChartName, h.ChartVersion()))
}

type OptionsMutator func(opts *Options)

type Options struct {
	Command []string
	Verbose bool
}

func WithExtraArgs(args ...string) OptionsMutator {
	return func(opts *Options) {
		opts.Command = append(opts.Command, args...)
	}
}

// InstallGloo calls glooctl to install Gloo. This is where image variants are handled as well.
func (h *SoloTestHelper) InstallGloo(ctx context.Context, timeout time.Duration, options ...OptionsMutator) error {
	deploymentType := "gateway"
	log.Printf("installing gloo in [%s] mode to namespace [%s]", deploymentType, h.InstallNamespace)
	glooctlCommand := []string{
		filepath.Join(h.BuildAssetDir, h.GlooctlExecName),
		"install", deploymentType,
		"--release-name", h.HelmChartName,
	}
	if h.LicenseKey != "" {
		glooctlCommand = append(glooctlCommand, "enterprise", "--license-key", h.LicenseKey)
	}
	if h.ReleasedVersion != "" {
		glooctlCommand = append(glooctlCommand, "-n", h.InstallNamespace, "--version", h.ReleasedVersion)
	} else {
		glooctlCommand = append(glooctlCommand,
			"-n", h.InstallNamespace,
			"-f", h.ChartPath())
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
		options = append(options, WithExtraArgs("--values", variantValuesFile))
	}

	io := &Options{
		Command: glooctlCommand,
		Verbose: true,
	}
	for _, opt := range options {
		opt(io)
	}

	if err := glooctlInstallWithTimeout(h.RootDir, io, timeout); err != nil {
		return errors.Wrapf(err, "error running glooctl install command")
	}

	return nil
}

func runWithTimeout(rootDir string, opts *Options, timeout time.Duration, operation string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	errChan := make(chan error, 1)
	go func() {
		err := exec.RunCommand(rootDir, opts.Verbose, opts.Command...)
		if err != nil {
			errChan <- errors.Wrapf(err, "error while executing gloo %s", operation)
		}
		errChan <- nil
	}()

	select {
	case err := <-errChan:
		return err // can be nil
	case <-ctx.Done():
		return fmt.Errorf("timed out while executing gloo %s", operation)
	}
}

// Wait for the glooctl install command to respond, err on timeout.
// The command returns as soon as certgen completes and all other
// deployments have been applied, which should only be delayed if
// there's an issue pulling the certgen docker image.
// Without this timeout, it would just hang indefinitely.
func glooctlInstallWithTimeout(rootDir string, opts *Options, timeout time.Duration) error {
	return runWithTimeout(rootDir, opts, timeout, "install")
}

// Upgrades Gloo via a helm upgrade. It returns a method that rolls-back helm to the version prior to this upgrade
func (h *SoloTestHelper) UpgradeGloo(ctx context.Context, timeout time.Duration, options ...OptionsMutator) (revertFunc func() error, err error) {
	log.Printf("upgrading gloo in namespace [%s]", h.InstallNamespace)

	revision, err := h.CurrentGlooRevision()
	if err != nil {
		return nil, err
	}

	helmCommand := []string{
		"helm",
		"upgrade",
		h.HelmChartName,
		h.ChartPath(),
		"-n", h.InstallNamespace,
		"--history-max",
		"0",
	}

	if h.Verbose {
		helmCommand = append(helmCommand, "-v")
	}

	opts := &Options{
		Command: helmCommand,
		Verbose: true,
	}
	for _, opt := range options {
		opt(opts)
	}

	if err := upgradeGlooWithTimeout(h.RootDir, opts, timeout); err != nil {
		return nil, errors.Wrapf(err, "error running glooctl install command")
	}

	return func() error {
		return h.RevertGlooUpgrade(ctx, timeout, WithExtraArgs([]string{
			strconv.Itoa(revision),
		}...))
	}, nil
}

func (h *SoloTestHelper) CurrentGlooRevision() (int, error) {
	command := []string{
		"bash",
		"-c",
		fmt.Sprintf("helm -n %s ls -o json | jq '.[] | select(.name=\"%s\") | .revision' | tr -d '\"'", h.InstallNamespace, h.HelmChartName),
	}
	out, err := exec.RunCommandOutput(h.RootDir, false, command...)
	if err != nil {
		return 0, errors.Wrapf(err, "error while fetching gloo revision")
	}
	return strconv.Atoi(strings.TrimSpace(out))
}

func upgradeGlooWithTimeout(rootDir string, opts *Options, timeout time.Duration) error {
	return runWithTimeout(rootDir, opts, timeout, "upgrade")
}

// Rollback Gloo. The version can be passed via the ExtraArgs option. If not specified it rolls-back to the previous version
// Eg: RevertGlooUpgrade(ctx, timeout, WithExtraArgs([]string{revision}))
func (h *SoloTestHelper) RevertGlooUpgrade(ctx context.Context, timeout time.Duration, options ...OptionsMutator) error {
	log.Printf("reverting gloo upgrade in namespace [%s]", h.InstallNamespace)
	helmCommand := []string{
		"helm",
		"rollback",
		h.HelmChartName,
		"-n", h.InstallNamespace,
		"--history-max",
		"0",
	}

	if h.Verbose {
		helmCommand = append(helmCommand, "-v")
	}

	opts := &Options{
		Command: helmCommand,
		Verbose: true,
	}
	for _, opt := range options {
		opt(opts)
	}

	if err := upgradeGlooWithTimeout(h.RootDir, opts, timeout); err != nil {
		return errors.Wrapf(err, "error running glooctl install command")
	}
	return nil
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
	log.Printf("uninstalling gloo...")
	cmdArgs := []string{
		filepath.Join(h.BuildAssetDir, h.GlooctlExecName), "uninstall", "-n", h.InstallNamespace, "--delete-namespace", "--release-name", h.HelmChartName,
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
