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
	udsEnabled := settings.GetDiscovery().GetUdsOptions().GetEnabled()
	fdsEnabled := settings.GetDiscovery().GetFdsMode() != v1.Settings_DiscoveryOptions_DISABLED
	if !udsEnabled && !fdsEnabled {
		contextutils.LoggerFrom(opts.WatchOpts.Ctx).
			Warn("Discovery (discovery.enabled) is enabled, but both UDS " +
				"(discovery.udsOptions.enabled) and FDS (discovery.fdsMode) are disabled. " +
				"While in this state, the discovery pod will be blocked. Consider disabling " +
				"discovery, or enabling one of the discovery features.")
	}
}
