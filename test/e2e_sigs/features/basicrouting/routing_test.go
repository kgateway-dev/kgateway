//go:build e2e

package basicrouting

import (
	"context"
	"fmt"
	"testing"

	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/test/e2e_sigs/assertions"
)

const (
	listenerHighPort = 8080
	listenerLowPort  = 80
)

func TestGatewayWithRoute(t *testing.T) {
	var gatewayAddress string

	feat := features.New("Gateway with Route").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			addr, err := getGatewayAddress(ctx, cfg)
			if err != nil {
				t.Fatalf("failed to get gateway address: %v", err)
			}
			gatewayAddress = addr
			return ctx
		}).
		Assess("successful response on all listeners", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			for _, port := range []int{listenerHighPort, listenerLowPort} {
				assertions.AssertSuccessfulResponse(t, gatewayAddress, port)
			}
			return ctx
		}).
		Feature()

	testenv.Test(t, feat)
}

func getGatewayAddress(ctx context.Context, cfg *envconf.Config) (string, error) {
	gw := &gwv1.Gateway{}
	if err := cfg.Client().Resources().Get(ctx, "test-gateway", "kgateway-test", gw); err != nil {
		return "", err
	}

	if len(gw.Status.Addresses) == 0 {
		return "", fmt.Errorf("gateway has no addresses in status")
	}

	address := gw.Status.Addresses[0].Value
	if address == "" {
		return "", fmt.Errorf("gateway address is empty")
	}

	return address, nil
}
