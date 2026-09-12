//go:build e2e

package dfp

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// gatewayWithRouteManifest contains the DFP Backend and the HTTPRoute that targets it
	gatewayWithRouteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "common.yaml")

	// connectTerminationManifest contains the CONNECT route and the TrafficPolicy that
	// terminates CONNECT on it
	connectTerminationManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "connect-termination.yaml")
)
