package register

import (
	"context"
	"os"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/skv2/pkg/multicluster/register"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var glooFederationPolicyRules = []v1.PolicyRule{
	{
		Verbs:     []string{"*"},
		APIGroups: []string{"gloo.solo.io", "gateway.solo.io"},
		Resources: []string{"*"},
	},
	{
		Verbs:     []string{"get", "list", "watch"},
		APIGroups: []string{"apps"},
		Resources: []string{"deployments", "daemonsets"},
	},
	{
		Verbs:     []string{"get", "list", "watch"},
		APIGroups: []string{""},
		Resources: []string{"pods", "nodes"},
	},
}

func Register(opts *options.Options) error {
	ctx := context.TODO()
	registerOpts := opts.Hub.Cluster.Register

	remoteConfigPath := registerOpts.RemoteKubeConfig
	remoteContext := registerOpts.RemoteContext

	remoteConfig, err := GetConfigWithContext("", remoteConfigPath, remoteContext)
	if err != nil {
		return err
	}

	registrantOpts := register.Options{
		ClusterName: registerOpts.ClusterName,
		RemoteCtx:   remoteContext,
		// TODO parameterize
		Namespace:       registerOpts.FederationNamespace,
		RemoteNamespace: registerOpts.TargetNamespace,
	}

	rbacOptions := register.RbacOptions{
		Options: registrantOpts,
		Roles:   nil,
		ClusterRoles: []*v1.ClusterRole{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: registrantOpts.Namespace,
					Name:      "gloo-federation-controller",
				},
				Rules: glooFederationPolicyRules,
			},
		},
	}

	registrant, err := register.DefaultRegistrant("", registerOpts.LocalClusterDomainOverride)
	if err != nil {
		return err
	}

	return register.RegisterClusterFromConfig(ctx, remoteConfig, rbacOptions, registrant)
}

func GetConfigWithContext(masterURL, kubeconfigPath, context string) (clientcmd.ClientConfig, error) {
	verifiedKubeConfigPath := clientcmd.RecommendedHomeFile
	if kubeconfigPath != "" {
		verifiedKubeConfigPath = kubeconfigPath
	}

	if err := assertKubeConfigExists(verifiedKubeConfigPath); err != nil {
		return nil, err
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = verifiedKubeConfigPath
	configOverrides := &clientcmd.ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{Server: masterURL}}

	if context != "" {
		configOverrides.CurrentContext = context
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides), nil
}

// expects `path` to be nonempty
func assertKubeConfigExists(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}

	return nil
}
