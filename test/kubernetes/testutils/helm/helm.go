package helm

import (
	"log"
	"os"
	"path/filepath"
	"time"

	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/go-utils/testutils/exec"
)

type InstallOptions struct {
	// If true, helm will be run with a -v flag
	Verbose bool

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
	// Install a released version of gloo. This is the value of the github tag that may have a leading 'v'
	ReleasedVersion string

	// ExtraArgs are additional arguments to pass to the helm command
	ExtraArgs []string

	// The version of the Helm chart. Calculated from either the chart or the released version. It will not have a leading 'v'
	Version string
}

func GetHelmOptions(testHelper *helper.SoloTestHelper) InstallOptions {
	return InstallOptions{
		RootDir:               testHelper.RootDir,
		TestAssetDir:          testHelper.TestAssetDir,
		BuildAssetDir:         testHelper.BuildAssetDir,
		HelmChartName:         testHelper.HelmChartName,
		HelmRepoIndexFileName: testHelper.HelmRepoIndexFileName,
		InstallNamespace:      testHelper.InstallNamespace,
		Version:               testHelper.GetVersion(),
	}
}

// Installs Gloo via "helm upgrade -i" with the given options
func HelmUpgradeInstallGloo(options InstallOptions) error {
	log.Printf("installing gloo via helm to namespace [%s]", options.InstallNamespace)
	helmCommand := []string{
		"helm", "upgrade", "--install", "gloo",
	}

	if options.ReleasedVersion != "" {
		helmCommand = append(helmCommand, "-n", options.InstallNamespace, "--version", options.ReleasedVersion)
	} else {
		helmCommand = append(helmCommand,
			"-n", options.InstallNamespace,
			filepath.Join(options.TestAssetDir, options.HelmChartName+"-"+options.Version+".tgz"))
	}

	variant := os.Getenv("IMAGE_VARIANT")
	if variant != "" {
		variantValuesFile, err := helper.GenerateVariantValuesFile(variant)
		if err != nil {
			return err
		}
		helmCommand = append(helmCommand, "--values", variantValuesFile)
	}

	if options.ExtraArgs != nil {
		helmCommand = append(helmCommand, options.ExtraArgs...)
	}

	if err := helmUpgradeInstallWithTimeout(options.RootDir, helmCommand, options.Verbose, time.Minute*2); err != nil {
		return errors.Wrapf(err, "error running helm upgrade install command")
	}

	return nil
}

// Wait for the helm install command to respond, err on timeout.
func helmUpgradeInstallWithTimeout(rootDir string, command []string, verbose bool, timeout time.Duration) error {
	runResponse := make(chan error, 1)
	go func() {
		err := exec.RunCommand(rootDir, verbose, command...)
		if err != nil {
			runResponse <- errors.Wrapf(err, "error while installing helm chart: %v", command)
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
