//go:build e2e

package quota

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	rlqsServerManifest  = quotaTestFile("rlqs-server.yaml")
	quotaPolicyManifest = quotaTestFile("quota-policy.yaml")
)

func quotaTestFile(filename string) string {
	return filepath.Join(fsutils.MustGetThisDir(), "testdata", filename)
}
