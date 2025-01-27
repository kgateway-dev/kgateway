package gloogateway

import (
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/test/testutils"
)

// Context contains the set of properties for a given installation of Gloo Gateway
type Context struct {
	InstallNamespace string

	// ProfileValuesManifestFile points to the file that contains the set of Helm values for a given profile
	// This is intended to represent a set of "production recommendations" and is defined as a standalone
	// file, to guarantee that tests specify a file that contains these values
	// For a test to define Helm values that are unique to the test, use ValuesManifestFile
	ProfileValuesManifestFile string

	// ValuesManifestFile points to the file that contains the set of Helm values that are unique to this test
	ValuesManifestFile string

	// whether or not the K8s Gateway controller is enabled
	K8sGatewayEnabled bool

	// whether or not the installation is an enterprise installation
	IsEnterprise bool

	// whether or not the validation webhook is configured to always accept resources,
	// i.e. if this is set to true, the webhook will accept regardless of errors found during validation
	ValidationAlwaysAccept bool

	// is populated if the installation has any AWS options configured (via `settings.aws.*` Helm values)
	AwsOptions *AwsOptions

	// TestAssetDir is the directory holding the test assets. Must be relative to RootDir.
	TestAssetDir string

	// Helm chart name
	HelmChartName string

	// Name of the helm index file name
	HelmRepoIndexFileName string

	// Install a released version of Gloo. This is the value of the github tag that may have a leading 'v'
	ReleasedVersion string

	// The version of the Helm chart. Calculated from either the chart or the released version. It will not have a leading 'v'
	ChartVersion string

	// The path to the local helm chart used for testing. Based on the TestAssertDir and relative to RootDir.
	ChartUri string
}

// AWS options that the installation was configured with
type AwsOptions struct {
	// corresponds to the `settings.aws.enableServiceAccountCredentials` helm value
	EnableServiceAccountCredentials bool

	// corresponds to the `settings.aws.stsCredentialsRegion` helm value
	StsCredentialsRegion string
}

// ValidateGlooGatewayContext returns an error if the provided Context is invalid
func ValidateGlooGatewayContext(context *Context) error {
	return ValidateContext(context, validateGlooGatewayValuesManifest)
}

func validateGlooGatewayValuesManifest(name string, file string) error {
	if file == "" {
		return eris.Errorf("%s must be provided in glooGateway.Context", name)
	}

	values, err := testutils.BuildHelmValues(testutils.HelmValues{ValuesFile: file})
	if err != nil {
		return eris.Wrapf(err, "failed to build helm values for %s", name)
	}
	err = testutils.ValidateHelmValues(values)
	if err != nil {
		return eris.Wrapf(err, "failed to validate helm values for %s", name)
	}
	return nil
}

// ValidateContext returns an error if the provided Context is invalid
// This accepts a manifestValidator so that it can be used by Gloo Gateway Enterprise
func ValidateContext(context *Context, manifestValidator func(string, string) error) error {
	// We are intentionally restrictive, and expect a ProfileValuesManifestFile to be defined.
	// This is because we want all existing and future tests to rely on this concept
	if err := manifestValidator("ProfileValuesManifestFile", context.ProfileValuesManifestFile); err != nil {
		return err
	}

	if err := manifestValidator("ValuesManifestFile", context.ValuesManifestFile); err != nil {
		return err
	}

	return nil
}
