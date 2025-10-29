//go:build e2e

package cors

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

var (
	// manifests
	simpleServiceManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "service.yaml")
	commonManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "common.yaml")
	httpRoutesManifest     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproutes.yaml")
	corsHttpRoutesManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproutes-cors.yaml")

	// traffic policies with cors configuration
	gwCorsTrafficPolicyManifest    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tp-gw-cors.yaml")
	routeCorsTrafficPolicyManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tp-route-cors.yaml")

	// objects created by deployer after applying gateway manifest
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	setup = base.TestCase{
		Manifests: []string{
			testdefaults.CurlPodManifest,
			simpleServiceManifest,
			commonManifest,
		},
	}

	testCases = map[string]*base.TestCase{
		"TestTrafficPolicyCorsForRoute": {
			Manifests: []string{httpRoutesManifest, routeCorsTrafficPolicyManifest},
		},
		"TestTrafficPolicyCorsAtGatewayLevel": {
			Manifests: []string{httpRoutesManifest, gwCorsTrafficPolicyManifest},
		},
		"TestTrafficPolicyRouteCorsOverrideGwCors": {
			Manifests: []string{httpRoutesManifest, gwCorsTrafficPolicyManifest, routeCorsTrafficPolicyManifest},
		},
		"TestHttpRouteCorsInRouteRules": {
			Manifests: []string{httpRoutesManifest, corsHttpRoutesManifest},
		},
		"TestHttpRouteAndTrafficPolicyCors": {
			Manifests: []string{httpRoutesManifest, corsHttpRoutesManifest, gwCorsTrafficPolicyManifest},
		},
	}
)
