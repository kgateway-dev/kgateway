package gloogateway

// Context contains the set of properties for a given installation of Gloo Gateway
type Context struct {
	InstallNamespace string

	ValuesManifestFile string
}
