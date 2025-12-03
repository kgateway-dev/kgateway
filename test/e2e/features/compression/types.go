//go:build e2e

package compression

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

var (
	// manifests
	commonManifest             = filepath.Join(fsutils.MustGetThisDir(), "testdata", "common.yaml")
	httpRoutesManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproutes.yaml")
	routeCompressionManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tp-route-compression.yaml")
	routeDecompressionManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tp-route-decompression.yaml")

	// objects created by deployer after applying gateway manifest
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	setup = base.TestCase{
		Manifests: []string{
			testdefaults.CurlPodManifest,
			testdefaults.HttpbinManifest,
			commonManifest,
		},
	}

	testCases = map[string]*base.TestCase{
		"TestTrafficPolicyResponseCompressionForRoute": {
			Manifests: []string{httpRoutesManifest, routeCompressionManifest},
		},
		"TestNoCompressionWithoutAcceptEncoding": {
			Manifests: []string{httpRoutesManifest, routeCompressionManifest},
		},
		"TestRequestDecompression": {
			Manifests: []string{httpRoutesManifest, routeCompressionManifest, routeDecompressionManifest},
		},
	}
)
