package helpers

import (
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

func WriteDefaultGateways(writeNamespace string, gatewayClient v1.GatewayClient) error {
	defaultGateway := defaults.DefaultGateway(writeNamespace)
	defaultSslGateway := defaults.DefaultSslGateway(writeNamespace)

	_, err := gatewayClient.Write(defaultGateway, clients.WriteOpts{})
	if err != nil {
		return err
	}
	_, err = gatewayClient.Write(defaultSslGateway, clients.WriteOpts{})

	return err
}
