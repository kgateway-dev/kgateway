//go:build e2e

package common

import (
	"context"
	"os"
	"testing"
	"time"

	"istio.io/istio/pkg/log"
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

// SetupBaseConfig applies the given base manifests and registers their teardown.
//
// Manifests are applied one file at a time, in the order given. A manifest that defines a
// namespace must therefore be listed before any manifest that creates resources in that namespace,
// so the namespace exists before those resources are applied. (Resources within a single file are
// applied in document order by the Istio client, so a self-contained file that declares a namespace
// and its resources together is always safe.)
func SetupBaseConfig(ctx context.Context, t *testing.T, installation *e2e.TestInstallation, manifests ...string) {
	for _, s := range log.Scopes() {
		s.SetOutputLevel(log.DebugLevel)
	}
	// Register cleanup before applying so partially applied manifests are still removed.
	//
	// Teardown does not wait for the namespaces to finish deleting: that takes close to a minute and
	// nothing after a passing run needs them gone. The next apply into them (a gotestsum rerun, or a
	// back-to-back local run) waits below instead, and only if they are still terminating.
	testutils.Cleanup(t, func() {
		// Delete through the Istio client so resources whose manifests omit metadata.namespace
		// resolve to the same default namespace the apply used.
		if err := installation.ClusterContext.IstioClient.DeleteYAMLFiles("", manifests...); err != nil {
			t.Logf("failed to delete base config manifests %v: %v", manifests, err)
		}
	})
	namespaces, err := namespacesFromManifests(manifests...)
	if err != nil {
		t.Fatalf("read namespaces from base config manifests: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	waited, err := waitForTerminatingNamespaces(waitCtx, installation.ClusterContext.Client, namespaces)
	if err != nil {
		t.Fatalf("wait for terminating base config namespaces %v: %v", namespaces, err)
	}
	if waited > 0 {
		t.Logf("waited %s for base config namespaces %v to finish terminating", waited.Round(time.Millisecond), namespaces)
	}
	// Apply manifests one at a time so a namespace file completes before the files that put
	// resources in it . Each apply is retried briefly to ride out transient API server errors
	// rather than hard-failing the whole test on a one-off failure.
	for _, manifest := range manifests {
		err := retry.UntilSuccess(func() error {
			return installation.ClusterContext.IstioClient.ApplyYAMLFiles("", manifest)
		}, retry.Timeout(1*time.Minute), retry.BackoffDelay(50*time.Millisecond))
		if err != nil {
			t.Fatalf("apply manifest %s: %v", manifest, err)
		}
	}
}

// SetupSharedNginxBackend applies the shared nginx pod (ns nginx-shared)
func SetupSharedNginxBackend(ctx context.Context, t *testing.T, installation *e2e.TestInstallation) {
	SetupBaseConfig(ctx, t, installation, testdefaults.NginxPodManifest)
	installation.AssertionsT(t).EventuallyPodsRunning(ctx, SharedNginxNamespace, metav1.ListOptions{
		LabelSelector: testdefaults.WellKnownAppLabel + "=nginx",
	}, 2*time.Minute)
}

// NewGateway resolves the LB address of the named Gateway from its status.
//
// GATEWAY_PORT_FORWARD: when set to true, requests go through port-forwards to the Gateway's
// Service instead, one per destination port, opened on first use and closed when t ends. This
// exists for hosts that cannot reach LB IPs, e.g. kind on macOS.
func NewGateway(ctx context.Context, t *testing.T, installation *e2e.TestInstallation, name types.NamespacedName) Gateway {
	gw := Gateway{
		NamespacedName: name,
		Address:        installation.AssertionsT(t).EventuallyGatewayAddress(ctx, name.Name, name.Namespace),
	}
	if os.Getenv("GATEWAY_PORT_FORWARD") == "true" {
		gw.forwards = newPortForwards(installation.ClusterContext.KubeContext, name)
		testutils.Cleanup(t, gw.forwards.close)
	}
	return gw
}

// SetupBaseGateway resolves the named Gateway with NewGateway and stores it in BaseGateway.
//
// GATEWAY_ADDRESS_OVERRIDE: when set, overrides the resolved address. This exists to support
// environments where the LB IP is not directly reachable from the host (e.g., k3d on macOS using
// port mapping). The override is applied ONLY here — single-gateway suites that use BaseGateway
// pick it up automatically. Suites that construct their own common.Gateway values (e.g.,
// multi-gateway suites that need more than one address) do NOT honor the override, since a single
// env var cannot disambiguate multiple gateways; GATEWAY_PORT_FORWARD covers those when they use
// NewGateway.
func SetupBaseGateway(ctx context.Context, t *testing.T, installation *e2e.TestInstallation, name types.NamespacedName) {
	BaseGateway = NewGateway(ctx, t, installation, name)
	if override := os.Getenv("GATEWAY_ADDRESS_OVERRIDE"); override != "" {
		BaseGateway.Address = override
	}
}

var BaseGateway Gateway
