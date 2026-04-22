//go:build e2e

package basicrouting_sigs

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"

	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/kgateway-dev/kgateway/v2/test/e2e_sigs/basicrouting/assertions"
)

func TestGatewayWithRoute(t *testing.T) {
	feat := features.New("Gateway with Route").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Get gateway address using kubectl
			cmd := exec.CommandContext(ctx, "kubectl", "get", "gateway", "basicrouting-gateway", "-n", "kgateway-test", "-o", "json")
			output, err := cmd.Output()
			if err != nil {
				t.Fatalf("failed to get gateway: %v", err)
			}

			var gw map[string]any
			if err := json.Unmarshal(output, &gw); err != nil {
				t.Fatalf("failed to parse gateway: %v", err)
			}

			status, ok := gw["status"].(map[string]any)
			if !ok {
				t.Fatal("gateway has no status")
			}

			addresses, ok := status["addresses"].([]any)
			if !ok || len(addresses) == 0 {
				t.Fatal("gateway has no addresses in status")
			}

			addr, ok := addresses[0].(map[string]any)
			if !ok {
				t.Fatal("invalid address format")
			}

			address, ok := addr["value"].(string)
			if !ok || address == "" {
				t.Fatal("gateway address is empty")
			}

			return context.WithValue(ctx, "gatewayAddress", address)
		}).
		Assess("successful response on all listeners", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			address := ctx.Value("gatewayAddress").(string)
			assertions.AssertSuccessfulResponse(t, address)
			return ctx
		}).
		Feature()

	testenv.Test(t, feat)
}
