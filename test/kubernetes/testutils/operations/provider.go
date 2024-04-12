package operations

type Provider interface {
	NewManifestOperation() Operation
}
