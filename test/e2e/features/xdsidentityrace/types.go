//go:build e2e

package xdsidentityrace

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

const (
	// The shared gateway proxy deployment created by the deployer for the base
	// "gateway" Gateway in kgateway-base.
	gwNamespace = "kgateway-base"
	gwName      = "gateway"
)

var (
	// manifests. The routes reference the shared `backend` Service that the base
	// TestKgateway setup deploys in kgateway-base — they exist only to force xDS
	// pushes, traffic is never sent through them.
	route1Manifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route1.yaml")
	// route2 is applied mid-test (not at BeforeTest) to force an xDS push onto the
	// already-established stream, so it is intentionally not part of any TestCase.
	route2Manifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route2.yaml")

	setup = base.TestCase{
		Manifests: []string{route1Manifest},
	}

	testCases = map[string]*base.TestCase{
		// route2 is applied by the test body, so no per-test manifests here.
		"TestReidentifyOnPodLabelDrift": {},
	}
)
