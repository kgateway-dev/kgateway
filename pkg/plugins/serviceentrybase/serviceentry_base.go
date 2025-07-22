package serviceentrybase

import "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/serviceentry"

type (
	Aliaser      = serviceentry.Aliaser
	PortMapper   = serviceentry.PortMapper
	Options      = serviceentry.Options
)

var (
	NewPluginWithOpts = serviceentry.NewPluginWithOpts
	HostnameAliaser   = serviceentry.HostnameAliaser
	DefaultPortMapper = serviceentry.DefaultPortMapper
)
