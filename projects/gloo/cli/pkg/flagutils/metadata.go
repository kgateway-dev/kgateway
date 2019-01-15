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

func AddNamespaceFlag(set *pflag.FlagSet, strptr *string) {
	set.StringVarP(strptr, "namespace", "n", defaults.GlooSystem, "namespace for reading or writing resources")
}
