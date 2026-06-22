//go:build e2e

package extproc

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// manifests
	setupManifest              = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	gatewayTargetRefManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway-targetref.yaml")
	httpRouteTargetRefManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproute-targetref.yaml")
	singleRouteManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "single-route.yaml")
	backendFilterManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "backend-filter.yaml")
	filterStageManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "filter-stage.yaml")
	filterStageWeightManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "filter-stage-weight.yaml")
	dualServersManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "extproc-dual-servers.yaml")
	deepMergeManifest          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "deep-merge.yaml")
	mixedStagesManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "mixed-stages.yaml")
)
