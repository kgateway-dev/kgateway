package version

import (
	"runtime"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

const (
	buildSubsystem = "build"
	versionLabel   = "version"
	revisionLabel  = "revision"
	goVersionLabel = "goversion"
	goOSLabel      = "goos"
	goArchLabel    = "goarch"
)

// registerBuildInfo publishes the controller's build information as kgateway_build_info,
// so the running version can be compared across installations from metrics alone.
func registerBuildInfo(v version) {
	metrics.NewInfo(
		metrics.GaugeOpts{
			Subsystem: buildSubsystem,
			Name:      "info",
			Help:      "Build information of the running controller. The series always has value 1.",
		},
		metrics.Label{Name: versionLabel, Value: v.Controller},
		metrics.Label{Name: revisionLabel, Value: v.Commit},
		metrics.Label{Name: goVersionLabel, Value: runtime.Version()},
		metrics.Label{Name: goOSLabel, Value: v.OS},
		metrics.Label{Name: goArchLabel, Value: v.Arch},
	)
}
