//go:build e2e

package common

import (
	"context"
	"os"
	"testing"

	"istio.io/istio/pkg/log"
	"istio.io/istio/pkg/test/util/assert"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
)

func SetupBaseConfig(ctx context.Context, t *testing.T, installation *e2e.TestInstallation, manifests ...string) {
	for _, s := range log.Scopes() {
		s.SetOutputLevel(log.DebugLevel)
	}
	err := installation.ClusterContext.IstioClient.ApplyYAMLFiles("", manifests...)
	assert.NoError(t, err)
}

// SetupBaseGateway resolves the LB address for the named Gateway and stores it in BaseGateway.
//
// GATEWAY_ADDRESS_OVERRIDE: when set, overrides the resolved address. This exists to support
// environments where the LB IP is not directly reachable from the host (e.g., k3d on macOS using
// port mapping). The override is applied ONLY here — single-gateway suites that use BaseGateway
// pick it up automatically. Suites that construct their own common.Gateway values (e.g.,
// multi-gateway suites that need more than one address) do NOT honor the override, since a single
// env var cannot disambiguate multiple gateways. Running such suites under k3d is out of scope.
func SetupBaseGateway(ctx context.Context, t *testing.T, installation *e2e.TestInstallation, name types.NamespacedName) {
	address := installation.AssertionsT(t).EventuallyGatewayAddress(
		ctx,
		name.Name,
		name.Namespace,
	)
	if override := os.Getenv("GATEWAY_ADDRESS_OVERRIDE"); override != "" {
		address = override
	}
	BaseGateway = Gateway{
		NamespacedName: name,
		Address:        address,
	}
}

var BaseGateway Gateway
