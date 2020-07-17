package flagutils

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/pflag"
)

const LocalClusterDomainOverride = "local-cluster-domain-override"

func AddRegisterFlags(set *pflag.FlagSet, register *options.Register) {
	set.StringVar(&register.ClusterName, "cluster-name", "", "name of the cluster to register")
	set.StringVar(&register.RemoteKubeConfig, "remote-kubeconfig", "", "path to the kubeconfig from which the registered cluster will be accessed")
	set.StringVar(&register.RemoteContext, "remote-context", "", "name of the kubeconfig context to use for registration")
	set.StringVar(&register.RemoteNamespace, "remote-namespace", "gloo-system", "namespace in the target cluster where registration artifacts should be written")
	set.StringVar(&register.LocalClusterDomainOverride, LocalClusterDomainOverride, "", "Swap out the domain of the remote cluster's k8s API server for the value of this flag; used mainly for debugging locally in docker, where you may provide a value like 'host.docker.internal'")
}

func AddUnregisterFlags(set *pflag.FlagSet, register *options.Unregister) {
	set.StringVar(&register.ClusterName, "cluster-name", "", "name of the cluster to register")
}

func AddClusterFlags(set *pflag.FlagSet, register *options.Cluster) {
	set.StringVar(&register.FederationNamespace, "federation-namespace", "gloo-fed", "namespace of the Gloo Federation control plane")
}
