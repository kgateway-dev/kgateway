package envoy

// DefaultImage is the default Envoy proxy image used by the controller.
const DefaultImage = "quay.io/solo-io/envoy-gloo:1.34.1-patch3"

// Image is the Envoy proxy image used by the controller.
// This is set by the linker during build.
var Image = DefaultImage
