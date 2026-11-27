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
	// ext_authz reading the body with the buffer filter staged ahead of it, and the same policy at
	// the buffer filter's default placement
	bufferedRouteManifest             = getTestFile("buffered-route.yaml")
	bufferedRouteDefaultStageManifest = getTestFile("buffered-route-default-stage.yaml")
	// JWT + gRPC ext-auth pair used to prove metadataContextNamespaces forwards the verified JWT payload
	metadataContextNamespacesManifest = getTestFile("metadata-context-namespaces.yaml")
)

func getTestFile(filename string) string {
	return filepath.Join(fsutils.MustGetThisDir(), "testdata", filename)
}
