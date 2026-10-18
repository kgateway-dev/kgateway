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

var buildInfo = metrics.NewGauge(
	metrics.GaugeOpts{
		Subsystem: buildSubsystem,
		Name:      "info",
		Help:      "Build information of the running controller. The series always has value 1.",
	},
	[]string{versionLabel, revisionLabel, goVersionLabel, goOSLabel, goArchLabel},
)

// RecordBuildInfo publishes the controller's build information as kgateway_build_info,
// so the running version can be compared across installations from metrics alone.
func RecordBuildInfo() {
	if !metrics.Active() {
		return
	}
	info := current()
	buildInfo.Reset()
	buildInfo.Set(1, []metrics.Label{
		{Name: versionLabel, Value: info.Controller},
		{Name: revisionLabel, Value: info.Commit},
		{Name: goVersionLabel, Value: runtime.Version()},
		{Name: goOSLabel, Value: info.OS},
		{Name: goArchLabel, Value: info.Arch},
	}...)
}

// current returns the version details collected in init. ref stays nil when the
// binary has no embedded build info, so fall back to what is known at runtime.
func current() version {
	if ref == nil {
		return version{Controller: UndefinedVersion, OS: runtime.GOOS, Arch: runtime.GOARCH}
	}
	return *ref
}
