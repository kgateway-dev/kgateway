package flagutils

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/pflag"
)

func AddVirtualServiceFlags(set *pflag.FlagSet, vs *options.InputVirtualService) {
	addDisplayNameFlag(set, &vs.DisplayName)
	addDomainsFlag(set, &vs.Domains)
}

func addDisplayNameFlag(set *pflag.FlagSet, ptr *string) {
	set.StringVar(ptr, "display-name", "", "descriptive name of virtual service (defaults to resource name)")
}

func addDomainsFlag(set *pflag.FlagSet, ptr *[]string) {
	set.StringSliceVar(ptr, "domains", []string{}, "comma separated list of domains")
}
