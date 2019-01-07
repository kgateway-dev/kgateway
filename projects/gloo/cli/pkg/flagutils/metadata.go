package flagutils

import (
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/log"
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
	// attempt to get current config context and use that as the default
	defaultNs, err := getNamespaceFromCurrentContext()
	if err != nil {
		log.Warnf("failed to retrieve kubeconfig namespace, falling back to %v", defaultNs)
		defaultNs = defaults.GlooSystem
	}

	set.StringVarP(strptr, "namespace", "n", defaultNs, "namespace for reading or writing resources")
}

func getNamespaceFromCurrentContext() (string, error) {
	kubeCfg, err := kubeutils.GetKubeConfig("", "")
	if err != nil {
		return "", err
	}
	if kubeCfg.CurrentContext == "" {
		return "", errors.Errorf("no current context set in kubeconfig")
	}
	kCtx, ok := kubeCfg.Contexts[kubeCfg.CurrentContext]
	if !ok {
		return "", errors.Errorf("ctx %v not found in kubeconfig, even though it's the current context", kubeCfg.CurrentContext)
	}
	return kCtx.Namespace, nil
}
