package version

const (
	EnterpriseTag = "0.18.23"

	// This value may change with new Gloo releases
	UiImageTag = EnterpriseTag

	// The following values should not change frequently
	UiImageRegistry        = "quay.io/solo-io"
	UiImageRepositoryProxy = "grpcserver-envoy"
	UiImageRepositoryFront = "grpcserver-ui"
	UiImageRepositoryBack  = "grpcserver-ee"
)
