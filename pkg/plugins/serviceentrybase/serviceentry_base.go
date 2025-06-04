package serviceentrybase

import "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/serviceentry"

type (
	Aliaser = serviceentry.Aliaser
	Options = serviceentry.Options
)

var (
	NewPluginWithOpts = serviceentry.NewPluginWithOpts
	HostnameAliaser   = serviceentry.HostnameAliaser
)
