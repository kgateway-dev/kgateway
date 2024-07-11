package gloogateway

// Context contains the set of properties for a given installation of Gloo Gateway
type Context struct {
	InstallNamespace string

	ValuesManifestFile string

	// whether or not the K8s Gateway controller is enabled
	K8sGatewayEnabled bool

	// whether or not the validation webhook is configured to always accept resources,
	// i.e. if this is set to true, the webhook will accept regardless of errors found during validation
	ValidationAlwaysAccept bool

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
