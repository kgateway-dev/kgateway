package deployer

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
)

func NewGatewayDeployer(controllerName string, cli client.Client, gwParams *GatewayParameters) (*deployer.Deployer, error) {
	chart, err := LoadGatewayChart()
	if err != nil {
		return nil, err
	}
	return deployer.NewDeployer(
		controllerName, cli, chart, gwParams, GatewayReleaseNameAndNamespace), nil
}

func NewInferencePoolDeployer(controllerName string, cli client.Client) (*deployer.Deployer, error) {
	inferenceExt := &InferenceExtension{}
	chart, err := LoadInferencePoolChart()
	if err != nil {
		return nil, err
	}
	return deployer.NewDeployer(
		controllerName, cli, chart, inferenceExt, InferenceExtensionReleaseNameAndNamespace), nil
}
