package gloogateway

// Context contains the set of properties for a given installation of Gloo Gateway
type Context struct {
	InstallNamespace string

	ValuesManifestFile string

	// SkipGlooInstall is a flag that indicates whether to skip the install of Gloo.
	// This is used to test against an existing installation of Gloo so that the
	// test framework does not need to install/uninstall Gloo Gateway.
	SkipGlooInstall bool
}
