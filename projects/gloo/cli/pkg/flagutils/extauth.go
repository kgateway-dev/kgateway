package flagutils

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/pflag"
)

func AddConfigFlagsOIDCSettings(set *pflag.FlagSet, oidc *options.OIDCSettings) {
	set.StringVar(&oidc.ExtAtuhServerUpstreamRef.Name, "extauth-server-name", "", "name of the ext auth server upstream")
	set.StringVar(&oidc.ExtAtuhServerUpstreamRef.Namespace, "extauth-server-namespace", "", "namespace of the ext auth server upstream")
}
