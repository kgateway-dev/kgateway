//go:build e2e

package compression

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

var (
	// manifests
	serviceManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "service.yaml")
	httpRoutesManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproutes.yaml")
	routeCompressionManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tp-route-compression.yaml")
	routeDecompressionManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tp-route-decompression.yaml")

	// proxy object meta for the shared gateway
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gateway",
		Namespace: "kgateway-base",
	}

	setup = base.TestCase{
		Manifests: []string{serviceManifest},
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
