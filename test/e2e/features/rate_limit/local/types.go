//go:build e2e

package local_rate_limit

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// local rate limit traffic policies
	routeLocalRateLimitManifest         = getTestFile("route-local-rate-limit.yaml")
	gwLocalRateLimitManifest            = getTestFile("gw-local-rate-limit.yaml")
	disabledRouteLocalRateLimitManifest = getTestFile("route-local-rate-limit-disabled.yaml")
	httpRoutesManifest                  = getTestFile("httproutes.yaml")
	extensionRefManifest                = getTestFile("extensionref-rl.yaml")
)

func getTestFile(filename string) string {
	return filepath.Join(fsutils.MustGetThisDir(), "testdata", filename)
}
