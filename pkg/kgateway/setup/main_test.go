package setup_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/kgateway-dev/kgateway/v2/test/envtestassets"
)

func TestMain(m *testing.M) {
	// These tests drive live in-process ADS servers. Run them with the xDS
	// first-connect grace period disabled (unless the environment explicitly
	// sets one): the delay is a mitigation, not a correctness requirement, so
	// the suite's eventually-consistent assertions must hold against the raw
	// reconnect race the delay merely narrows — running at zero both surfaces
	// bugs the grace period would mask and removes a fixed per-stream second
	// from the suite's runtime. The delay contract itself is pinned by
	// TestFirstConnectDelayGatesFirstRequestPerStream in pkg/krtcollections.
	// The variable is read lazily on first stream connect, so setting it here
	// (after package initialization, before any server runs) is effective.
	if os.Getenv("KGW_XDS_FIRST_CONNECT_DELAY") == "" {
		os.Setenv("KGW_XDS_FIRST_CONNECT_DELAY", "0")
	}

	// Start a single envtest environment (etcd + kube-apiserver) shared by all
	// tests in this package; booting one per test dominates the suite runtime.
	assetsDir, err := envtestassets.GetEnvTestAssetsDir()
	if err != nil {
		fmt.Printf("failed to get assets dir: %v\n", err)
		os.Exit(1)
	}
	sharedTestEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "crds"),
			filepath.Join("..", "..", "..", "install", "helm", "kgateway-crds", "templates"),
			filepath.Join("testdata", "istio_crds_setup"),
		},
		ErrorIfCRDPathMissing: true,
		// set assets dir so we can run without the makefile
		BinaryAssetsDirectory: assetsDir,
		// This often hangs (for unknown reasons); we don't need cleanup so just kill it almost instantly
		ControlPlaneStopTimeout: time.Millisecond,
		// web hook to add cluster ips to services
	}
	if _, err := sharedTestEnv.Start(); err != nil {
		fmt.Printf("failed to start envtest: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	sharedTestEnv.Stop()
	os.Exit(code)
}
