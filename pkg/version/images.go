package version

const (
	// This value may change with new Gloo releases
	UiImageTag = "0.18.22"

	// The following values should not change frequently
	UiImageRegistry        = "quay.io/solo-io"
	UiImageRepositoryProxy = "grpcserver-envoy"
	UiImageRepositoryFront = "grpcserver-ui"
	UiImageRepositoryBack  = "grpcserver-ee"
)
