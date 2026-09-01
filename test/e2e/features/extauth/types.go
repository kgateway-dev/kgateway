//go:build e2e

package extauth

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

// Manifest files
var (
	// route to the shared base echo backend
	routeManifest                = getTestFile("route.yaml")
	gatewayWithRouteManifest     = getTestFile("common.yaml")
	extAuthManifest              = getTestFile("ext-authz-server.yaml")
	securedGatewayPolicyManifest = getTestFile("secured-gateway-policy.yaml")
	securedRouteManifest         = getTestFile("secured-route.yaml")
	insecureRouteManifest        = getTestFile("insecure-route.yaml")
)

func getTestFile(filename string) string {
	return filepath.Join(fsutils.MustGetThisDir(), "testdata", filename)
}
