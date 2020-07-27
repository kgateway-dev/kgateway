package register

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/skv2/pkg/multicluster/register"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var glooFederationPolicyRules = []v1.PolicyRule{
	{
		Verbs:     []string{"*"},
		APIGroups: []string{"gloo.solo.io", "gateway.solo.io", "enterprise.gloo.solo.io", "ratelimit.solo.io"},
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
		Resources: []string{"pods", "nodes", "services"},
	},
}

func Register(opts *options.Options) error {
	ctx := context.Background()
	registerOpts := opts.Cluster.Register

	clusterRegisterOpts := register.RegistrationOptions{
		RemoteKubeCfgPath:     registerOpts.RemoteKubeConfig,
		RemoteKubeContext:     registerOpts.RemoteContext,
		ClusterDomainOverride: registerOpts.LocalClusterDomainOverride,
		ClusterName:           registerOpts.ClusterName,
		Namespace:             opts.Cluster.FederationNamespace,
		RemoteNamespace:       registerOpts.RemoteNamespace,
		ClusterRoles: []*v1.ClusterRole{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: registerOpts.RemoteNamespace,
					Name:      "gloo-federation-controller",
				},
				Rules: glooFederationPolicyRules,
			},
		},
	}

	return clusterRegisterOpts.RegisterCluster(ctx)
}
