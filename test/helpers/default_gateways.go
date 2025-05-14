//go:build ignore

package helpers

import (
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	v1 "github.com/kgateway-dev/kgateway/v2/internal/gateway/pkg/api/v1"
	"github.com/kgateway-dev/kgateway/v2/internal/gateway/pkg/defaults"
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
