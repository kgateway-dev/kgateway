//go:build e2e

package basicrouting_sigs

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
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
	gomega.RegisterTestingT(t)

	var gatewayAddress string

	feat := features.New("Gateway with Route").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			gw := &gwv1.Gateway{}
			err := cfg.Client().Resources().Get(ctx, "test-gateway", "kgateway-test", gw)
			if err != nil {
				t.Fatalf("failed to get gateway: %v", err)
			}

			if len(gw.Status.Addresses) == 0 {
				t.Fatal("gateway has no addresses in status")
			}

			address := gw.Status.Addresses[0].Value
			if address == "" {
				t.Fatal("gateway address is empty")
			}

			gatewayAddress = address
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
