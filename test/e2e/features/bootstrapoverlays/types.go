//go:build e2e

package bootstrapoverlays

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

var (
	// manifests
	gatewayManifest           = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway.yaml")
	paramsOverlayManifest     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "params-overlay.yaml")
	paramsManagedPathManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "params-managed-path.yaml")
	paramsNoneManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "params-none.yaml")

	// proxyObjectMeta names the Gateway, its proxy Deployment and Service, and the
	// GatewayParameters that hold its bootstrap overlays.
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "bootstrap-overlays",
		Namespace: "default",
	}

	setup = base.TestCase{
		Manifests: []string{defaults.HttpbinManifest},
	}

	// Each test supplies the GatewayParameters its Gateway references.
	testCases = map[string]*base.TestCase{
		"TestOverlayApplied": {
			Manifests: []string{paramsOverlayManifest, gatewayManifest},
		},
		"TestManagedPathModes": {
			Manifests: []string{paramsManagedPathManifest, gatewayManifest},
		},
		"TestFragmentFeatureFlag": {
			Manifests: []string{paramsNoneManifest, gatewayManifest},
		},
		"TestStaticNamePrefix": {
			Manifests: []string{paramsOverlayManifest, gatewayManifest},
		},
	}
)
