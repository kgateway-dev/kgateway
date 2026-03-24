package deployer

import (
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
)

func NewGatewayDeployer(controllerName string, scheme *runtime.Scheme, client apiclient.Client, gwParams *GatewayParameters, opts ...deployer.Option) (*deployer.Deployer, error) {
	envoyChart, err := LoadEnvoyChart()
	if err != nil {
		return nil, err
	}
	return deployer.NewDeployer(
		controllerName, scheme, client, envoyChart, gwParams, GatewayReleaseNameAndNamespace, opts...), nil
}
