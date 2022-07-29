package utils

import (
	"errors"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

// ErrorIfDiscoveryServiceUnused returns an error if the discovery service is not used by any upstreams.
// if discovery is enabled, but both UDS & FDS are disabled, we should error loudly as the
// discovery pod is being deployed for no reason.
func ErrorIfDiscoveryServiceUnused(settings *v1.Settings) error {
	udsEnabled := GetUdsEnabled(settings)
	fdsEnabled := GetFdsEnabled(settings)
	if !udsEnabled && !fdsEnabled {
		return errors.New("discovery (discovery.enabled) is enabled, but both UDS " +
			"(discovery.udsOptions.enabled) and FDS (discovery.fdsMode) are disabled. " +
			"While in this state, the discovery pod will be no-op. Consider disabling " +
			"discovery, or enabling one of the discovery features")
	}
	return nil
}

func GetUdsEnabled(settings *v1.Settings) bool {
	if settings == nil || settings.GetDiscovery().GetUdsOptions().GetEnabled() == nil {
		return true
	}
	return settings.GetDiscovery().GetUdsOptions().GetEnabled().GetValue()
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

func GetWatchLabels(settings *v1.Settings) map[string]string {
	return settings.GetDiscovery().GetUdsOptions().GetWatchLabels()
}
