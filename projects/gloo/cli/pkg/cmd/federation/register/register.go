package register

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/skv2/pkg/multicluster/register"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Register(opts *options.Options) error {
	ctx := context.Background()
	registerOpts := opts.Cluster.Register

	clusterRegisterOpts := register.RegistrationOptions{
		APIServerAddress: registerOpts.LocalClusterDomainOverride,
		ClusterName:      registerOpts.ClusterName,
		Namespace:        opts.Cluster.FederationNamespace,
		RemoteNamespace:  registerOpts.RemoteNamespace,
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
