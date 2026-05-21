//go:build e2e

package basicrouting

import (
	"context"
	"testing"

	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/kgateway-dev/kgateway/v2/test/e2e_sigs_framework/assertions"
	"github.com/kgateway-dev/kgateway/v2/test/e2e_sigs_framework/common/gateway"
)

const (
	listenerHighPort = 8080
	listenerLowPort  = 80
)

func TestGatewayWithRoute(t *testing.T) {
	var gatewayAddress string

	feat := features.New("Gateway with Route").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			addr, err := gateway.GetAddress(ctx, cfg, "test-gateway", "kgateway-test")
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
