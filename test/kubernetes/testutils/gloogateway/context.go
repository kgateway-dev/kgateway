package gloogateway

// Context contains the set of properties for a given installation of Gloo Gateway
type Context struct {
	InstallNamespace string

	ValuesManifestFile string

	// whether or not the validation webhook is configured to always accept resources,
	// i.e. if this is set to true, the webhook will accept regardless of errors found during validation
	ValidationAlwaysAccept bool
}
