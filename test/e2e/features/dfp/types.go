//go:build e2e

package dfp

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

// gatewayWithRouteManifest contains the DFP Backend and the HTTPRoute that targets it
var gatewayWithRouteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "common.yaml")
