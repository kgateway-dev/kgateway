//go:build e2e

package global_rate_limit

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// paths to test manifests
	commonManifest            = getTestFile("common.yaml")
	httpRoutesManifest        = getTestFile("routes.yaml")
	ipRateLimitManifest       = getTestFile("ip-rate-limit.yaml")
	pathRateLimitManifest     = getTestFile("path-rate-limit.yaml")
	userRateLimitManifest     = getTestFile("user-rate-limit.yaml")
	combinedRateLimitManifest = getTestFile("combined-rate-limit.yaml")
	rateLimitServerManifest   = getTestFile("rate-limit-server.yaml")
)

func getTestFile(filename string) string {
	return filepath.Join(fsutils.MustGetThisDir(), "testdata", filename)
}
