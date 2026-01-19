//go:build e2e

package buffer

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

var (
	// manifests
	commonManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "common.yaml")
	httpRouteManifest     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproute.yaml")
	trafficPolicyManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "trafficpolicy.yaml")

	// objects created by deployer after applying gateway manifest
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	setup = base.TestCase{
		Manifests: []string{
			testdefaults.CurlPodManifest,
			testdefaults.HttpEchoPodManifest,
			commonManifest,
		},
	}

	testCases = map[string]*base.TestCase{
		"TestBufferLimit": {
			Manifests: []string{httpRouteManifest, trafficPolicyManifest},
		},
	}
)
