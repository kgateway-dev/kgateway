package flagutils

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/spf13/pflag"
)

func AddMetadataFlags(set *pflag.FlagSet, metaptr *core.Metadata) {
	addNameFlag(set, &metaptr.Name)
	AddNamespaceFlag(set, &metaptr.Namespace)
}

func addNameFlag(set *pflag.FlagSet, strptr *string) {
	set.StringVar(strptr, "name", "", "name of the resource to read or write")
}

// DefaultNamespace wraps defaults.GlooSystem to separate global Gloo defaults from glooctl flags
var DefaultNamespace = defaults.GlooSystem

func AddNamespaceFlag(set *pflag.FlagSet, strptr *string) {
	set.StringVarP(strptr, "namespace", "n", DefaultNamespace, "namespace for reading or writing resources")
}

func AddExcludeCheckFlag(set *pflag.FlagSet, strarrptr *[]string) {
	set.StringSliceVarP(strarrptr, "exclude", "x", []string{}, "check to exclude: (deployments, pods, upstreams, upstreamgroup, auth-configs, rate-limit-configs, secrets, virtual-services, gateways, proxies, xds-metrics)")
}
