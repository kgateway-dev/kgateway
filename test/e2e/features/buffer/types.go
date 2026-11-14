//go:build e2e

package buffer

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

var testCases = map[string]*base.TestCase{
	"TestBufferLimit": {
		Manifests: []string{filepath.Join(fsutils.MustGetThisDir(), "testdata", "trafficpolicy.yaml")},
	},
}
