package constants

const (
	GlooGatewayEnableK8sGwControllerEnv = "GG_EXPERIMENTAL_K8S_GW_CONTROLLER"

	// GlooGatewayDeployerImage is an experimental API that allows users to inject the image
	// that the deployer should provision. The value should be in the form:
	//	[image_repository:image_tag] --> quay.io/solo-io/gloo-envoy-wrapper:1.17.0-beta1
	// This API is intended to be short-lived, as the long-term vision is to support a CR to define
	// configuration for our deployer
	GlooGatewayDeployerImage = "GG_EXPERIMENTAL_DEPLOYER_IMAGE"
)
