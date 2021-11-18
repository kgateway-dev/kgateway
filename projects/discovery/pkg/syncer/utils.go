package syncer

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/go-utils/contextutils"
)

// If discovery is enabled, but both UDS & FDS are disabled, the discovery pod will not return
// from projects/discovery/cmd/main.go's run() function (it will hang waiting for an error to
// be emitted from either UDS or FDS)
func LogIfDiscoveryServiceUnused(opts *bootstrap.Opts) {
	settings := opts.Settings
	udsEnabled := GetUdsEnabled(settings)
	fdsEnabled := GetFdsEnabled(settings)
	if !udsEnabled && !fdsEnabled {
		contextutils.LoggerFrom(opts.WatchOpts.Ctx).
			Warn("Discovery (discovery.enabled) is enabled, but both UDS " +
				"(discovery.udsOptions.enabled) and FDS (discovery.fdsMode) are disabled. " +
				"While in this state, the discovery pod will be blocked. Consider disabling " +
				"discovery, or enabling one of the discovery features.")
	}
}

func GetUdsEnabled(settings *v1.Settings) bool {
	if settings == nil || settings.GetDiscovery() == nil || settings.GetDiscovery().GetUdsOptions() == nil {
		return true
	}
	return settings.GetDiscovery().GetUdsOptions().GetEnabled()
}

func GetFdsMode(settings *v1.Settings) v1.Settings_DiscoveryOptions_FdsMode {
	if settings == nil || settings.GetDiscovery() == nil {
		return v1.Settings_DiscoveryOptions_WHITELIST
	}
	return settings.GetDiscovery().GetFdsMode()
}

func GetFdsEnabled(settings *v1.Settings) bool {
	return GetFdsMode(settings) != v1.Settings_DiscoveryOptions_DISABLED
}
