package flagutils

import (
	"github.com/solo-io/gloo/v2/pkg/defaults"
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

func AddPodSelectorFlag(set *pflag.FlagSet, strptr *string) {
	set.StringVarP(strptr, "pod-selector", "p", "gloo", "Label selector for pod scanning")
}

func AddResourceNamespaceFlag(set *pflag.FlagSet, strptr *[]string) {
	set.StringArrayVarP(strptr, "resource-namespaces", "r", []string{}, "Namespaces in which to scan gloo custom resources. If not provided, all watched namespaces (as specified in settings) will be scanned.")
}

func AddExcludeCheckFlag(set *pflag.FlagSet, strarrptr *[]string) {
	set.StringSliceVarP(strarrptr, "exclude", "x", []string{}, "check to exclude: (deployments, pods, upstreams, upstreamgroup, auth-configs, rate-limit-configs, secrets, virtual-services, gateways, proxies, xds-metrics)")
}

// AddReadOnlyFlag adds a flag to our flag set that indicates we shouldn't do anything that requires RBAC create permissions
func AddReadOnlyFlag(set *pflag.FlagSet, readOnly *bool) {
	set.BoolVarP(readOnly, "read-only", "", false, "only do checks that dont require creating resources (i.e. port forwards)")
}

// AddKubeContextFlag adds a flag to our flag set that sets the kube context we use when interacting with kubernetes
func AddKubeContextFlag(set *pflag.FlagSet, KubeContext *string) {
	set.StringVarP(KubeContext, "kube-context", "", "", "kube context to use when interacting with kubernetes")
}
