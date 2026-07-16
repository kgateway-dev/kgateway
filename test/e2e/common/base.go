//go:build e2e

package common

import (
	"context"
	"os"
	"testing"
	"time"

	"istio.io/istio/pkg/log"
	"istio.io/istio/pkg/test/util/assert"
	"istio.io/istio/pkg/test/util/retry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// SharedNginxNamespace is the namespace of the shared nginx backend applied by
// SetupSharedNginxBackend.
const SharedNginxNamespace = "nginx-shared"

func SetupBaseConfig(ctx context.Context, t *testing.T, installation *e2e.TestInstallation, manifests ...string) {
	for _, s := range log.Scopes() {
		s.SetOutputLevel(log.DebugLevel)
	}
	// Register cleanup before applying so partially applied manifests are still removed.
	testutils.Cleanup(t, func() {
		if err := installation.ClusterContext.IstioClient.DeleteYAMLFiles("", manifests...); err != nil {
			t.Logf("failed to delete base config manifests %v: %v", manifests, err)
		}
	})
	// Retry the apply: a gotestsum rerun starts a fresh process while the previous
	// process's cleanup may still be deleting the namespaces these manifests create,
	// and applying into a terminating namespace is rejected by the API server.
	err := retry.UntilSuccess(func() error {
		return installation.ClusterContext.IstioClient.ApplyYAMLFiles("", manifests...)
	}, retry.Timeout(2*time.Minute), retry.Delay(2*time.Second))
	assert.NoError(t, err)
}

// SetupSharedNginxBackend applies the shared nginx pod (ns nginx-shared)
func SetupSharedNginxBackend(ctx context.Context, t *testing.T, installation *e2e.TestInstallation) {
	SetupBaseConfig(ctx, t, installation, testdefaults.NginxPodManifest)
	installation.AssertionsT(t).EventuallyPodsRunning(ctx, SharedNginxNamespace, metav1.ListOptions{
		LabelSelector: testdefaults.WellKnownAppLabel + "=nginx",
	}, 2*time.Minute)
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
